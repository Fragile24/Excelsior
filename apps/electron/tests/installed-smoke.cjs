const assert=require("node:assert/strict");
const fs=require("node:fs");
const os=require("node:os");
const path=require("node:path");
const http=require("node:http");
const net=require("node:net");
const {spawn}=require("node:child_process");
const {once}=require("node:events");
const pause=ms=>new Promise(r=>setTimeout(r,ms));
async function until(fn,label){let last;for(let i=0;i<150;i++){try{const value=await fn();if(value)return value;}catch(e){last=e;}await pause(200);}throw new Error("Timed out: "+label+(last?" "+last:""));}
async function port(){const s=net.createServer();s.listen(0,"127.0.0.1");await once(s,"listening");const p=s.address().port;await new Promise(r=>s.close(r));return p;}
async function main(){
 const executable=process.argv[2];if(!executable)throw new Error("Pass the unpacked application executable");
 const root=fs.mkdtempSync(path.join(os.tmpdir(),"excelsior-desktop-smoke-"));
 const workspace=path.join(root,"workspace");fs.mkdirSync(workspace);
 const provider=http.createServer((req,res)=>{
  req.resume();req.on("end",()=>{
   res.writeHead(200,{"Content-Type":"text/event-stream"});
   const chunk=(delta,finish=null)=>({id:"smoke",object:"chat.completion.chunk",created:1,model:"deepseek-v4-flash",choices:[{index:0,delta,finish_reason:finish}]});
   res.write("data: "+JSON.stringify(chunk({role:"assistant",content:"Smoke answer saved successfully"}))+"\n\n");
   res.write("data: "+JSON.stringify(chunk({},"stop"))+"\n\n");res.end("data: [DONE]\n\n");
  });
 });provider.listen(0,"127.0.0.1");await once(provider,"listening");
 const debugPort=await port(),enginePort=await port();
 const child=spawn(path.resolve(executable),["--smoke-test","--remote-debugging-port="+debugPort,"--user-data-dir="+path.join(root,"profile")],{windowsHide:true,stdio:["ignore","pipe","pipe"],env:{...process.env,EXCELSIOR_ENGINE:"",EXCELSIOR_ENGINE_TOKEN:"",EXCELSIOR_AUTO_ENGINE:"1",EXCELSIOR_ENGINE_ADDR:"127.0.0.1:"+enginePort,EXCELSIOR_WORKSPACE:workspace,EXCELSIOR_TOKEN_FILE:path.join(root,"config","owner-token"),DEEPSEEK_API_KEY:"smoke-local-only",DEEPSEEK_BASE_URL:"http://127.0.0.1:"+provider.address().port}});
 let logs="";child.stdout.on("data",b=>logs+=b);child.stderr.on("data",b=>logs+=b);
 let ws,evaluate;const errors=[];
 try{
  const target=await until(async()=>{const targets=await(await fetch("http://127.0.0.1:"+debugPort+"/json",{signal:AbortSignal.timeout(1000)})).json();return targets.find(t=>t.type==="page"&&t.url.startsWith("excelsior:"));},"packaged renderer");
  ws=new WebSocket(target.webSocketDebuggerUrl);await once(ws,"open");
  let seq=0;const pending=new Map();
  ws.addEventListener("message",event=>{const m=JSON.parse(event.data);if(m.id){const p=pending.get(m.id);pending.delete(m.id);m.error?p?.reject(new Error(m.error.message)):p?.resolve(m.result);}else if(m.method==="Runtime.exceptionThrown")errors.push(m.params.exceptionDetails.text);});
  const call=(method,params={})=>new Promise((resolve,reject)=>{const id=++seq;pending.set(id,{resolve,reject});ws.send(JSON.stringify({id,method,params}));});
  await call("Runtime.enable");
  evaluate=async expression=>{const r=await call("Runtime.evaluate",{expression,awaitPromise:true,returnByValue:true});if(r.exceptionDetails)throw new Error(JSON.stringify(r.exceptionDetails));return r.result.value;};
  await until(()=>evaluate('!!document.querySelector("textarea") && [...document.querySelectorAll("button")].some(b=>b.getAttribute("aria-label")?.startsWith("New session in workspace"))'),"authenticated workspace");
  assert.equal(await evaluate('document.body.innerText.includes("Smoke answer saved successfully")'),false);
  await evaluate('(()=>{const e=document.querySelector("textarea");Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype,"value").set.call(e,"Smoke prompt");e.dispatchEvent(new Event("input",{bubbles:true}));})()');
  await until(()=>evaluate('!document.querySelector(\'button[aria-label="Send message"]\')?.disabled'),"ready composer");
  await evaluate('document.querySelector(\'button[aria-label="Send message"]\').click()');
  await until(()=>evaluate('document.body.innerText.includes("Smoke answer saved successfully")'),"rendered response");
  await until(()=>{const dir=path.join(workspace,".excelsior","sessions");return fs.existsSync(dir)&&fs.readdirSync(dir).filter(f=>f.endsWith(".json")).some(f=>fs.readFileSync(path.join(dir,f),"utf8").includes("Smoke answer saved successfully"));},"durable result");
  await evaluate('[...document.querySelectorAll("button")].find(b=>b.getAttribute("aria-label")==="New session in workspace").click()');
  await until(()=>evaluate('!!document.querySelector("textarea") && !document.body.innerText.includes("Smoke answer saved successfully")'),"populated to empty transcript");
  assert.deepEqual(errors,[],"renderer exceptions");
  const exited=once(child,"exit");
  await evaluate('window.electronAPI.windowControl("close")');
  await Promise.race([exited,pause(15000).then(()=>{throw new Error("Desktop shutdown timed out");})]);
  await until(async()=>{try{await fetch("http://127.0.0.1:"+enginePort+"/health");return false;}catch{return true;}},"owned engine shutdown");
  console.log("PASS: packaged resources, authenticated readiness, mounted empty/populated/empty transcript, persisted response, owned shutdown");
 }catch(e){console.error(logs);console.error("Renderer",errors,evaluate?await evaluate("({text:document.body.innerText, scripts:[...document.scripts].map(s=>s.src),api:!!window.electronAPI})"):null);throw e;}finally{ws?.close();if(child.exitCode===null)child.kill();provider.close();}
}
main().catch(e=>{console.error(e);process.exitCode=1;});

