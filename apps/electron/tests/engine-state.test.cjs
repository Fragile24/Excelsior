require("./load-typescript.cjs");
const test=require("node:test");
const assert=require("node:assert/strict");
const fs=require("node:fs");
const path=require("node:path");
const {decodeMessage}=require("../lib/protocol.ts");
const {reduce,initialEngineState}=require("../lib/engineState.ts");
const wire=(type,payload,workspace="A")=>({ver:"v1",type,payload,workspace});
const apply=(s,type,p,ws)=>reduce(s,{type:"server",msg:wire(type,p,ws)});
const snap=(id="shared",runId="current")=>({id,runId,running:true,messages:[{role:"user",content:"hello"}],events:[],unsavedAvailable:false,projectionUnavailable:false});

test("Go lifecycle fixture decodes and failure survives terminal snapshot",()=>{
 const fixtures=JSON.parse(fs.readFileSync(path.resolve(__dirname,"../../../pkg/protocol/testdata/lifecycle.json"),"utf8"));
 let state=initialEngineState();
 for(const item of fixtures)state=reduce(state,{type:"server",msg:decodeMessage(JSON.stringify(item))});
 const conversation=state.conversations["session-one"];
 assert.equal(conversation.outcome.status,"persistence_failed");
 assert.equal(conversation.running,false);
 assert.equal(conversation.blocks[1].content,"generated result");
 assert.match(conversation.blocks.at(-1).content,/not saved/);
});
test("scope changes reset every server-derived map; stale run and interaction events are ignored",()=>{
 let s=apply(initialEngineState(),"workspace.set",{workspace:"A"});
 s=apply(s,"session.data",snap());
 const prompt={sessionId:"shared",runId:"current",interactionId:"latest",question:"choose",options:["yes"]};
 s=apply(s,"ask.req",prompt);
 assert.equal(apply(s,"interaction.done",{sessionId:"shared",runId:"old",interactionId:"old"}),s);
 assert.equal(apply(s,"delta",{sessionId:"shared",runId:"old",type:"text",text:"old"}),s);
 assert.equal(apply(s,"done",{sessionId:"shared",runId:"old",status:"canceled",persisted:false}),s);
 s=apply(s,"workspace.set",{workspace:"B"},"B");
 assert.equal(s.activeId,null);
 assert.deepEqual(s.conversations,{});
 assert.deepEqual(reduce(s,{type:"reset"}),initialEngineState());
});
test("malformed or incompatible messages fail before changing state",()=>{
 for(const msg of [
  {ver:"v0",type:"auth",payload:{}},
  wire("session.data",{...snap(),runId:""}),
  wire("session.data",{...snap(),events:[{sessionId:"other",runId:"current",type:"text"}]}),
  wire("done",{sessionId:"shared",runId:"current",status:"succeeded",persisted:false}),
  wire("ask.req",{sessionId:"shared",runId:"current",interactionId:"i",question:"?",options:42}),
  wire("worktree.list",{worktrees:[{path:"/repo",branch:"main",main:true}]}),
  wire("worktree.create",{path:"/repo",branch:"main",main:false,current:"no"}),
 ])assert.throws(()=>decodeMessage(JSON.stringify(msg)));
});
test("context token breakdown must contain nonnegative counts matching the exact input total",()=>{
 const payload={sessionId:"shared",runId:"current",type:"context",context:{
  model:"test-model",state:"ready",reason:"automatic",inputTokens:100,inputBudget:1000,
  outputReserve:100,windowTokens:1100,softLimit:800,hardLimit:900,tier:4,
  canCompact:true,beforeTokens:100,compactions:0,
  breakdown:{system:20,tools:30,user:25,assistant:25},
 }};
 assert.doesNotThrow(()=>decodeMessage(JSON.stringify(wire("delta",payload))));
 assert.throws(()=>decodeMessage(JSON.stringify(wire("delta",{...payload,context:{...payload.context,breakdown:{...payload.context.breakdown,user:-1}}}))));
 assert.throws(()=>decodeMessage(JSON.stringify(wire("delta",{...payload,context:{...payload.context,breakdown:{...payload.context.breakdown,assistant:24}}}))));
});
test("terminal snapshots replace the view without duplicate text or error blocks",()=>{
 let s=apply(initialEngineState(),"workspace.set",{workspace:"A"});
 const p={...snap(),running:false,messages:[{role:"assistant",content:"answer"}],outcome:{sessionId:"shared",runId:"current",status:"persistence_failed",persisted:false,error:"disk full"},unsavedAvailable:true};
 s=apply(s,"session.data",p);s=apply(s,"session.data",p);
 assert.equal(s.conversations.shared.blocks.length,2);
 assert.equal(s.conversations.shared.blocks[0].content,"answer");
});

test("parallel results match tool IDs even when identical tools complete out of order",()=>{
 let s=apply(initialEngineState(),"workspace.set",{workspace:"A"});
 s=apply(s,"session.data",snap());
 const delta=p=>{s=apply(s,"delta",{sessionId:"shared",runId:"current",toolName:"read_file",...p});};
 delta({type:"tool_start",toolCallID:"one",toolArgs:'{"path":"one"}'});
 delta({type:"tool_start",toolCallID:"two",toolArgs:'{"path":"two"}'});
 delta({type:"tool_result",toolCallID:"two",toolResult:"second result"});
 delta({type:"tool_result",toolCallID:"one",toolResult:"first result"});
 const tools=s.conversations.shared.blocks.filter(b=>b.role==="tool");
 assert.deepEqual(tools.map(b=>[b.callId,b.args,b.content]),[
  ["one",'{"path":"one"}',"first result"],["two",'{"path":"two"}',"second result"],
 ]);
});

test("a session snapshot replaces all run state without selecting a background session",()=>{
 let s=apply(initialEngineState(),"workspace.set",{workspace:"A"});
 s=reduce(s,{type:"active",id:"foreground"});
 s=apply(s,"session.data",{...snap("background"),pending:{type:"ask.req",payload:{sessionId:"background",runId:"current",interactionId:"question",question:"?",options:[]}}});
 s=apply(s,"delta",{sessionId:"background",runId:"current",type:"generation",totalTokens:5});
 const previous=s.conversations.background;
 s=apply(s,"session.data",{...snap("background","next"),running:false});
 assert.equal(s.activeId,"foreground");
 assert.equal(previous.pending.payload.interactionId,"question");
 assert.equal(previous.usage.total,5);
 assert.equal(s.conversations.background.pending,null);
 assert.equal(s.conversations.background.outcome,null);
 assert.equal(s.conversations.background.usage.total,0);
 assert.equal(s.conversations.background.running,false);
});

test("tool-call IDs can repeat across turns without replacing older results or arguments",()=>{
 let s=apply(initialEngineState(),"workspace.set",{workspace:"A"});
 const messages=[];
 for(const path of ["old.go","previous.go"]){
  messages.push({role:"assistant",tool_calls:[{id:"reused",type:"function",function:{name:"read_file",arguments:JSON.stringify({path})}}]});
  messages.push({role:"tool",name:"read_file",tool_call_id:"reused",content:path});
 }
 s=apply(s,"session.data",{...snap(),messages});
 const history=s.conversations.shared.blocks;
 assert.deepEqual(history.map(b=>JSON.parse(b.args).path),["old.go","previous.go"]);
 for(const delta of [
  {type:"tool_start",toolArgs:'{"path":"new.go"}'},
  {type:"tool_result",toolResult:"new content"},
 ])s=apply(s,"delta",{sessionId:"shared",runId:"current",toolName:"read_file",toolCallID:"reused",...delta});
 assert.deepEqual(s.conversations.shared.blocks.slice(0,2),history);
 assert.equal(s.conversations.shared.blocks[2].content,"new content");
 assert.equal(JSON.parse(s.conversations.shared.blocks[2].args).path,"new.go");
});

