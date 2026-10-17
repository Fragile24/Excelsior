require("./load-typescript.cjs");
const test=require("node:test");
const assert=require("node:assert/strict");
const {accentInk,readAccentColor,readReasoningEffort,readTranscriptSize,readWorkspaces}=require("../lib/usePreferences.ts");
const {workspaceName}=require("../lib/format.ts");

test("workspace preferences keep unique paths and preserve saved folder locations",()=>{
 assert.deepEqual(readWorkspaces(JSON.stringify(["/work/one",{id:"old",name:"One",path:"/work/one"},{path:"/work/two"},{name:"placeholder"},null,42," "])),["/work/one","/work/two"]);
 for(const raw of [null,"broken",'{}','"wrong type"'])assert.deepEqual(readWorkspaces(raw),[]);
 assert.equal(workspaceName("C:\\work\\Excelsior"),"Excelsior");
 assert.equal(workspaceName("/work/another/"),"another");
});

test("accent preferences validate colors and keep button text readable",()=>{
 assert.equal(readAccentColor("#AbCdEF"),"#abcdef");
 for(const raw of [null,"red","#fff","#12345678"])assert.equal(readAccentColor(raw),null);
 assert.equal(accentInk("#ffffff"),"#202020");
 assert.equal(accentInk("#000000"),"#ffffff");
});

test("transcript size preference accepts only supported sizes",()=>{
 assert.equal(readTranscriptSize("18"),18);
 assert.equal(readTranscriptSize("11"),14);
 assert.equal(readTranscriptSize("large"),14);
});

test("reasoning preference accepts supported effort levels and defaults safely",()=>{
 for(const effort of ["none","low","high","max"])assert.equal(readReasoningEffort(effort),effort);
 for(const raw of [null,"medium","invalid"])assert.equal(readReasoningEffort(raw),"high");
});
