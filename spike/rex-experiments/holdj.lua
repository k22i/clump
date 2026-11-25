local term = "com.superlogical.terminal"
local n = tonumber(rex.args.n or 30)
local dt = tonumber(rex.args.dt or 0.033)
for i = 1, n do
  rex.block.call(term, "write", { block_id = rex.args.block, data = rex.args.key or "j" })
  rex.sleep(dt)
end
return n
