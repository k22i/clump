-- Experiment: split to the right of an anchor block, giving the command at creation.
local term = "com.superlogical.terminal"
local anchor = rex.args.anchor
local root = rex.args.root
local label = "clump-watch:" .. root
local t0 = os.clock()
local res, err = rex.session.new_split{
  anchor_id = anchor, direction = "horizontal", side = "after", ratio = 0.5, focus = false,
  layout = rex.layout.block{
    flavor = "com.superlogical.terminal.shell",
    label = label,
    options = { cwd = root, command = { "sh", "-c", "echo WATCH-PANE pwd=$(pwd) args=\"$*\"; exec sleep 600", "sh", "watch", "--repo", root } },
  },
}
if err then error("new_split: " .. tostring(err), 0) end
local new = res.block_ids[1]
rex.sleep(0.8)
-- find it again by label
local found
local lb = rex.session.list_blocks{}
for _, b in ipairs(lb.blocks) do if b.label == label then found = b.block_id end end
local screen = rex.block.call(term, "format", { block_id = new, trim = true }).content
local proc = rex.block.call(term, "process", { block_id = new })
local view = rex.session.view{}
return { new_split = res, found_by_label = found, same = (found == new), screen = screen,
         fg = proc.foreground and { argv0 = proc.foreground.argv0, name = proc.foreground.name, cwd = proc.foreground.cwd } }
