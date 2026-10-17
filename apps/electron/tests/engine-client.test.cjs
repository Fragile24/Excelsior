require("./load-typescript.cjs");
const test=require("node:test");
const assert=require("node:assert/strict");
const {EngineClient,CommandError}=require("../lib/engineClient.ts");
const {reduce,initialEngineState}=require("../lib/engineState.ts");
const pause=()=>new Promise(resolve=>setImmediate(resolve));
async function until(predicate){for(let i=0;i<100;i++){if(predicate())return;await new Promise(r=>setTimeout(r,5));}assert.fail("condition timed out");}
class Server {
 sockets=[];commands=[];sessions=new Map([["A",["one","background"]],["B",["one"]]]);worktrees=[{path:"A",branch:"main",head:"abc1234",main:true,current:true}];sequence=0;drop=null;pending=new Map();permissions=new Map();
 socket=()=>{
  const s={readyState:0,workspace:"A",send:raw=>this.receive(s,JSON.parse(raw)),close:()=>{if(s.readyState===3)return;s.readyState=3;queueMicrotask(()=>s.onclose?.());}};
  this.sockets.push(s);queueMicrotask(()=>{s.readyState=1;s.onopen?.();});return s;
 };
 emit(s,type,payload,id){queueMicrotask(()=>{if(s.readyState===1)s.onmessage?.({data:JSON.stringify({ver:"v1",workspace:s.workspace,type,payload,id})});});}
 receive(s,m){
  this.commands.push({...m,workspace:s.workspace});
  if(this.drop===m.type){this.drop=null;s.close();return;}
  let p={};let type=m.type;
  switch(type){
   case "auth":p={ok:true,workspace:"A",capabilities:["run-lifecycle-v1"]};break;
   case "workspace.set":if(m.payload.workspace==="bad"){this.emit(s,"error",{error:"invalid workspace",code:"command_failed"},m.id);return;}s.workspace=m.payload.workspace;p={workspace:s.workspace};break;
   case "settings.set":this.permissions.set(s.workspace,m.payload.permission);
   case "settings.get":p={permission:this.permissions.get(s.workspace)??"ask"};break;
   case "session.list":p={sessions:(this.sessions.get(s.workspace)??[]).map(id=>({id,title:id,count:0}))};break;
   case "worktree.list":p={worktrees:this.worktrees};break;
   case "worktree.create":p={path:"A-worktrees/"+m.payload.branch,branch:m.payload.branch,main:false,current:false};this.worktrees.push(p);break;
   case "session.create":p={id:"created-"+ ++this.sequence};this.sessions.get(s.workspace).push(p.id);break;
   case "session.data":case "chat.req":{
    type="session.data";const id=m.payload.id??m.payload.sessionId;const pending=this.pending.get(id);
    p={id,runId:pending?"run-"+id:(m.type==="chat.req"?"new-run":undefined),running:!!pending||m.type==="chat.req",messages:[],unsavedAvailable:false,projectionUnavailable:false};
    if(pending)p.pending={ver:"v1",type:"permission.req",payload:{sessionId:id,runId:"run-"+id,interactionId:"approval-"+id,tool:"edit"}};
    break;
   }
   case "permission.resp":case "ask.resp":p={accepted:true};break;
   case "chat.cancel":p={...m.payload,accepted:true};break;
   case "session.unsubscribe":p={id:m.payload.id};break;
   case "session.delete":p={deleted:m.payload.id};break;
   case "session.rename":p=m.payload;break;
   default:throw new Error("unexpected command "+type);
  }
  this.emit(s,type,p,m.id);
 }
}
async function connected(t){
 const server=new Server(),statuses=[],messages=[];let state=initialEngineState();
 const client=new EngineClient("ws://test",{getToken:async()=>"token",createSocket:server.socket,onSelection:id=>{state=reduce(state,{type:"active",id});},onStatus:s=>{statuses.push(s);state=reduce(state,{type:"status",status:s});},onMessage:msg=>{messages.push(msg);state=reduce(state,{type:"server",msg});},onError:e=>assert.fail(e)});
 t.after(()=>client.close());await until(()=>statuses.at(-1)==="connected");
 return{client,server,statuses,messages,state:()=>state};
}
test("authentication synchronizes scope, settings and snapshots before readiness",async t=>{
 const {server,statuses}=await connected(t);
 assert.deepEqual(statuses,["connecting","authenticating","synchronizing","connected"]);
 assert.deepEqual(server.commands.slice(0,5).map(m=>m.type),["auth","workspace.set","settings.get","session.list","session.data"]);
});
test("worktree commands list linked checkouts and acknowledge creation",async t=>{
 const {client,server}=await connected(t);
 assert.deepEqual(await client.listWorktrees(),[{path:"A",branch:"main",head:"abc1234",main:true,current:true}]);
 const created=await client.createWorktree("feature/example");
 assert.deepEqual(created,{path:"A-worktrees/feature/example",branch:"feature/example",main:false,current:false});
 assert.deepEqual(server.commands.slice(-2).map(command=>command.type),["worktree.list","worktree.create"]);
});

test("permission selection waits for acknowledgement and preserves the confirmed mode on rejection",async t=>{
 const {client,server,state}=await connected(t);
 assert.equal(state().permissionMode,"ask");
 const emit=server.emit.bind(server);let acknowledge;
 server.emit=(s,type,payload,id)=>{if(type==="settings.set")acknowledge=()=>emit(s,type,payload,id);else emit(s,type,payload,id);};
 const change=client.setSettings({permission:"deny"});
 assert.equal(state().permissionMode,"ask");
 acknowledge();await change;
 assert.equal(state().permissionMode,"deny");
 server.emit=(s,type,payload,id)=>emit(s,type==="settings.set"?"error":type,type==="settings.set"?{error:"Settings could not be saved",code:"command_failed"}:payload,id);
 await assert.rejects(client.setSettings({permission:"allow"}),e=>e.delivery==="rejected");
 assert.equal(state().permissionMode,"deny");
 await client.setWorkspace("B");
 assert.equal(state().permissionMode,"ask");
});
test("workspace rejection preserves scope; successful switch drops old projections",async t=>{
 const {client,state}=await connected(t);
 await assert.rejects(client.setWorkspace("bad"),e=>e instanceof CommandError&&e.delivery==="rejected");
 assert.equal(state().workspace,"A");
 await client.setWorkspace("B");
 assert.equal(state().workspace,"B");assert.deepEqual(Object.keys(state().conversations),["one"]);
});
test("new conversation waits for its correlated creation acknowledgement",async t=>{
 const {client,server}=await connected(t);
 await client.startChat("hello","model",null,false,"max");
 const create=server.commands.findIndex(m=>m.type==="session.create");
 const start=server.commands.findIndex(m=>m.type==="chat.req");
 assert.ok(start>create);
 assert.equal(server.commands[start].payload.sessionId,"created-1");
 assert.equal(server.commands[start].payload.reasoningEffort,"max");
 assert.ok(server.commands.every(m=>m.type==="auth"||typeof m.id==="string"));
 assert.equal(new Set(server.commands.filter(m=>m.id).map(m=>m.id)).size,server.commands.filter(m=>m.id).length);
});
test("unsent and uncertain requests are distinct, and no mutation is automatically replayed",async t=>{
 const {client,server}=await connected(t);
 server.drop="chat.req";
 await assert.rejects(client.startChat("hello","model","one"),e=>e.delivery==="uncertain");
 await assert.rejects(client.startChat("hello","model","one"),e=>e.delivery==="unsent");
 await new Promise(r=>setTimeout(r,1100));
 assert.equal(server.commands.filter(m=>m.type==="chat.req").length,1);
});
test("reconnect restores non-default workspace and background approval subscriptions",async t=>{
 const {client,server,state}=await connected(t);
 await client.setWorkspace("B");
 server.sessions.set("B",["one","background"]);
 server.pending.set("background",true);
 await client.selectSession("background");await client.selectSession("one");
 server.sockets.at(-1).close();
 await new Promise(r=>setTimeout(r,1100));
 await until(()=>server.sockets.length===2&&state().status==="connected");
 assert.equal(state().workspace,"B");
 assert.equal(state().conversations.background.pending.payload.interactionId,"approval-background");
 assert.equal(state().activeId,"one");
 const restored=server.commands.filter(m=>m.workspace==="B"&&m.type==="session.data").map(m=>m.payload.id);
 assert.ok(restored.filter(id=>id==="one").length>=2);
 assert.ok(restored.filter(id=>id==="background").length>=2);
});

test("the client selects only acknowledged sessions and refreshes metadata after a terminal event",async t=>{
 const {client,server,state}=await connected(t);
 await client.selectSession("background");
 assert.equal(state().activeId,"background");
 await client.startChat("hello","model","background");
 const count=server.commands.filter(m=>m.type==="session.list").length;
 server.emit(server.sockets.at(-1),"done",{sessionId:"background",runId:"new-run",status:"succeeded",persisted:true});
 await until(()=>server.commands.filter(m=>m.type==="session.list").length===count+1);
 assert.equal(state().conversations.background.running,false);
 assert.equal(state().activeId,"background");
 await client.deleteSession("background");
 assert.equal(state().activeId,null);
 assert.equal(state().conversations.background,undefined);
});
test("closed clients cannot emit late status or message callbacks",async t=>{
 const {client,server,statuses,messages}=await connected(t);const socket=server.sockets[0];
 client.close();const statusCount=statuses.length,messageCount=messages.length;
 socket.onclose();socket.onerror();socket.onmessage({data:JSON.stringify({ver:"v1",type:"workspace.set",workspace:"B",payload:{workspace:"B"}})});
 await pause();assert.equal(statuses.length,statusCount);assert.equal(messages.length,messageCount);
});


test("workspace change interrupts a dependent create-and-send operation",async t=>{
 const {client,server}=await connected(t);
 const emit=server.emit.bind(server);let held;
 server.emit=(s,type,payload,id)=>{if(type==="session.create")held=()=>emit(s,type,payload,id);else emit(s,type,payload,id);};
 const send=client.startChat("keep this draft","model",null);
 const rejected=assert.rejects(send,e=>e.delivery==="unsent");
 await until(()=>!!held);await client.setWorkspace("B");held();await rejected;
 assert.equal(server.commands.filter(m=>m.type==="chat.req").length,0);
 assert.equal(server.commands.filter(m=>m.workspace==="B"&&m.type==="session.data"&&m.payload.id==="created-1").length,0);
});
