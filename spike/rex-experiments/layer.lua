-- Experiment: label at creation vs OSC 2 title; floating layer with a command.
local term = "com.superlogical.terminal"
local s, err = rex.session.new_split{ anchor_id = rex.args.anchor, direction = "vertical", side = "after", focus = false,
  layout = rex.layout.block{ flavor = "com.superlogical.terminal.shell", label = "clump-watch:LABELTEST",
    options = { shell = "none", command = { "/bin/sh", "-c", "printf '\\033]2;some program title\\033\\\\'; exec sleep 600" } } } }
if err then error(err, 0) end
local l, lerr = rex.session.new_layer{ bounds = { 0.1, 0.1, 0.8, 0.8 }, focus = false, window_id = rex.args.window,
  layout = rex.layout.block{ flavor = "com.superlogical.terminal.shell", label = "clump-oneshot",
    options = { cwd = rex.args.root, command = { "sh", "-c", "echo FLOAT pwd=$(pwd); exec sleep 600" } } } }
if lerr then error(lerr, 0) end
rex.sleep(1)
local function label(id)
  for _, b in ipairs(rex.session.list_blocks{}.blocks) do if b.block_id == id then return b.label end end
end
return { split_label_after_title = label(s.block_ids[1]),
  split_title = rex.block.call(term, "title", { block_id = s.block_ids[1] }),
  layer = l, layer_screen = rex.block.call(term, "format", { block_id = l.block_ids[1], trim = true }).content,
  layer_label = label(l.block_ids[1]) }
