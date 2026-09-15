-- One atomic eligibility decision and claim. No business outcome is recorded.
local p=KEYS[1]
local q=cjson.decode(ARGV[1])
local function result(decision,message) return cjson.encode({decision=decision,message=message}) end
local saved=redis.call('GET',p..'grant:'..q.id)
if saved then
 local g=cjson.decode(saved)
 if g.owner~=q.owner or g.step~=q.step then return result('denied','Invocation belongs to another caller or step') end
 if q.cancel then
  if g.bound then return result('denied','Reported invocation must finish validation before it can be closed') end
  if g.decision=='allowed' then
   if redis.call('HGET',p..'active_invocations',q.step)==q.id then redis.call('HDEL',p..'active_invocations',q.step) end
   g.decision='cancelled'; redis.call('SET',p..'grant:'..q.id,cjson.encode(g),'EX',604800)
   redis.call('PUBLISH',p..'wait_changes','changed')
  end
  return result(g.decision,'Invocation closed; consumed prerequisites are not restored')
 end
 if g.decision~='allowed' then return result('denied','Invocation has already been consumed or cancelled') end
 if g.bound then return result('denied','Invocation has already been reported') end
 if redis.call('HGET',p..'meta','status')~='active' then return result('denied','Thread is not active') end
 return result('allowed','Permission already granted to this invocation')
end
if q.cancel then return result('denied','Unknown invocation') end
local state=redis.call('HGET',p..'meta','status')
if not state then return result('unavailable','Live thread state is unavailable') end
if state~='active' then return result('denied','Thread is '..state) end
if redis.call('HLEN',p..'pending_validation')>0 then
 for _,state in ipairs(redis.call('HVALS',p..'pending_validation')) do
  if state=='unavailable' then return result('unavailable','A recorded event could not be validated; reconcile it before requesting permission') end
 end
 return result('pending','Waiting for recorded events to finish validation')
end
if redis.call('HGET',p..'active_invocations',q.step) then return result('pending','Another invocation of this step is outstanding') end
-- Strict workflows serialize permission claims until their outcomes are validated.
if q.strict and redis.call('HLEN',p..'active_invocations')>0 then return result('pending','Another guarded transition is outstanding') end
local snapshots=redis.call('HGETALL',p..'successful_contexts')
local successful={}
local latest=nil
local latestOrder=0
for i=1,#snapshots,2 do
 local value=cjson.decode(snapshots[i+1]); local order=tonumber(value.order)
 if not order then return result('unavailable','Invalid successful step state') end
 successful[snapshots[i]]=order
 if order>latestOrder then latest=snapshots[i];latestOrder=order end
end
local expectedOrder=tonumber(redis.call('GET',p..'last_validated_success')) or 0
if expectedOrder>latestOrder then return result('unavailable','Successful validation snapshots are incomplete') end
local function contains(list,value)
 if type(list)~='table' then return false end
 for _,v in ipairs(list) do if v==value then return true end end
 return false
end
if not latest then
 -- Older cached history without snapshots is not evidence of an empty thread.
 if redis.call('ZCARD',p..'current_steps')>0 then return result('unavailable','Successful history needs live validation snapshots') end
 if not contains(q.entries,q.step) then return result('pending','Waiting for an entry step') end
elseif q.strict and not contains(q.transitions[latest],q.step) then
 return result('pending','Waiting for a permitted preceding step')
end
if type(q.required)=='table' then
 for _,dep in ipairs(q.required) do if not successful[dep] then return result('pending','Waiting for successful step '..dep) end end
end
local last=tonumber(redis.call('HGET',p..'invocation_order',q.step)) or 0
if type(q.fresh)=='table' then
 for _,dep in ipairs(q.fresh) do if (successful[dep] or 0)<=last then return result('pending','Waiting for a fresh successful invocation of '..dep) end end
end
local now=redis.call('TIME')
local sequence=tonumber(redis.call('HGET',p..'meta','successOrder')) or 0
local order=math.max(now[1]*1000000+now[2],latestOrder+1,last+1,sequence+1)
redis.call('HSET',p..'meta','successOrder',string.format('%.0f',order))
redis.call('HSET',p..'invocation_order',q.step,string.format('%.0f',order))
redis.call('HSET',p..'active_invocations',q.step,q.id)
redis.call('SET',p..'grant:'..q.id,cjson.encode({owner=q.owner,step=q.step,decision='allowed',order=string.format('%.0f',order)}))
return result('allowed','Flow prerequisites satisfied; invocation claimed')
