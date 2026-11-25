-- Experiment: rex.ui.pick inside an action, plus ctx and pwd of the focused block.
rex.action{
  name = "clump_spike_pick", title = "clump spike: pick test",
  run = function(ctx, args)
    local term = "com.superlogical.terminal"
    local pwd = rex.block.call(term, "pwd", { block_id = ctx.block_id })
    local t0 = os.time()
    local id, why, detail = rex.ui.pick{
      title = "clump spike test: pick anything (or Esc)", placeholder = "Filter",
      timeout = 20,
      items = {
        { id = "branch", title = "This branch vs main", subtitle = "default", icon = "branch" },
        { id = "abc1234", title = "abc1234 Fix the thing", subtitle = "2 days ago", icon = "tag", detail = "+12 -3" },
      },
    }
    return { ctx = ctx, pwd = pwd, picked = id, why = why, detail = detail, waited_s = os.time() - t0 }
  end,
}
