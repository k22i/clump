-- clump for Rex: semantic diff review next to lazygit.
--
-- Load it from ~/.config/rex/init.lua (`clump rex install` prints the line):
--
--   dofile("/Users/you/.local/share/clump/rex/init.lua")
--
-- Actions (Rex action names cannot contain dots, so the task's clump.pick,
-- clump.watch and clump.watch_close are spelled with underscores):
--
--   clump_pick         choose a commit or "this branch vs default" and open
--                      clump's one-shot TUI in a floating layer
--   clump_watch        open `clump watch` in a split to the right of the
--                      focused pane (lazygit), or focus the one already open
--                      for that repository
--   clump_watch_close  close the watch pane for the focused pane's repository
--
-- Default bindings (set CLUMP_NO_BINDINGS = true before dofile to skip them,
-- or rebind with rex.bind after it):
--
--   ctrl+alt+g  clump_pick
--   ctrl+alt+w  clump_watch
--   ctrl+alt+q  clump_watch_close
--
-- Set CLUMP_BIN before dofile when `clump` is not on the login shell's PATH.
--
-- Every action also takes optional args, for scripting and tests:
--   block = <block id>   the pane to act from (default: the focused block)
--   repo  = <path>       skip the cwd lookup and use this repository root

local BIN = rawget(_G, "CLUMP_BIN") or "clump"
local TERM = "com.superlogical.terminal"
local FLAVOR = "com.superlogical.terminal.shell"
local LABEL = "clump-watch:"

local M = { bin = BIN }

-- url_decode turns %XX escapes back into bytes.
local function url_decode(s)
  return (s:gsub("%%(%x%x)", function(h) return string.char(tonumber(h, 16)) end))
end

-- focused_block returns the focused block of the session's active window.
local function focused_block(session_id)
  local view = rex.session.view{ session_id = session_id }
  if not view then return nil end
  local active = view.active_window_id or view.focused_window_id
  for _, w in ipairs(view.windows or {}) do
    if (active == nil or w.window_id == active) and w.focused_block_id then
      return w.focused_block_id, w.window_id
    end
  end
  return nil
end

-- window_of returns the window holding block_id.
local function window_of(session_id, block_id)
  local lb = rex.session.list_blocks{ session_id = session_id }
  for _, b in ipairs((lb and lb.blocks) or {}) do
    if b.block_id == block_id then return b.window_id end
  end
  return nil
end

-- block_cwd is the terminal's working directory: its OSC 7 pwd (a file://
-- URL, empty until the shell reports one), else the foreground process cwd.
local function block_cwd(block_id)
  local p = rex.block.call(TERM, "pwd", { block_id = block_id })
  if p and p.pwd and p.pwd ~= "" then
    local path = p.pwd:match("^file://[^/]*(/.*)$") or p.pwd
    return url_decode(path)
  end
  local proc = rex.block.call(TERM, "process", { block_id = block_id })
  if proc and proc.foreground and proc.foreground.cwd and proc.foreground.cwd ~= "" then
    return proc.foreground.cwd
  end
  return nil
end

-- repo_root resolves the git top level of dir.
local function repo_root(dir)
  local r, err = rex.exec({ "git", "-C", dir, "rev-parse", "--show-toplevel" }, { timeout = 5 })
  if not r then return nil, err end
  if not r.ok then return nil, (r.stderr ~= "" and r.stderr or "not a git repository: " .. dir) end
  return (r.stdout:gsub("%s+$", ""))
end

-- context resolves the acting block and its repository from ctx and args.
local function context(ctx, args)
  args = args or {}
  local session = ctx.session_id
  local block = args.block or ctx.block_id
  if not block then block = focused_block(session) end
  if args.repo then return session, block, args.repo end
  if not block then error("clump: no focused pane to act from", 0) end
  local dir = block_cwd(block)
  if not dir then error("clump: cannot tell the focused pane's directory", 0) end
  local root, err = repo_root(dir)
  if not root then error("clump: " .. tostring(err), 0) end
  return session, block, root
end

-- find_watch returns the block id of the watch pane for root, if any.
local function find_watch(session, root)
  local lb = rex.session.list_blocks{ session_id = session }
  for _, b in ipairs((lb and lb.blocks) or {}) do
    if b.label == LABEL .. root then return b.block_id end
  end
  return nil
end

M.find_watch = find_watch

rex.action{
  name = "clump_watch",
  title = "clump: Watch lazygit",
  description = "Open `clump watch` to the right of the focused pane (lazygit), or focus the existing one for this repository.",
  category = "clump",
  keywords = "diff review group lazygit watch",
  args = { block = "string?", repo = "string?" },
  run = function(ctx, args)
    local session, block, root = context(ctx, args)
    local existing = find_watch(session, root)
    if existing then
      rex.session.focus_block{ session_id = session, block_id = existing }
      return { block_id = existing, created = false, repo = root }
    end
    if not block then error("clump: no pane to split", 0) end
    local res, err = rex.session.new_split{
      session_id = session,
      anchor_id = block,
      direction = "horizontal",
      side = "after",
      ratio = 0.5,
      focus = false,
      layout = rex.layout.block{
        flavor = FLAVOR,
        label = LABEL .. root,
        options = { cwd = root, command = { M.bin, "watch", "--repo", root } },
      },
    }
    if not res then error("clump: new_split: " .. tostring(err), 0) end
    return { block_id = res.block_ids[1], created = true, repo = root }
  end,
}

rex.action{
  name = "clump_watch_close",
  title = "clump: Close the watch pane",
  description = "Close the `clump watch` pane for the focused pane's repository.",
  category = "clump",
  args = { block = "string?", repo = "string?" },
  run = function(ctx, args)
    local session, block, root = context(ctx, args)
    local id = find_watch(session, root)
    if not id then return { closed = false, repo = root } end
    local _, err = rex.call("block.close", { session_id = session, block_id = id })
    if err then error("clump: block.close: " .. tostring(err), 0) end
    return { closed = true, block_id = id, repo = root }
  end,
}

-- open_oneshot runs clump with argv in a floating layer over the window.
local function open_oneshot(session, block, root, argv)
  local cmd = { M.bin }
  for _, a in ipairs(argv) do cmd[#cmd + 1] = a end
  local res, err = rex.session.new_layer{
    session_id = session,
    window_id = block and window_of(session, block) or nil,
    bounds = { 0.04, 0.04, 0.92, 0.92 },
    focus = true,
    layout = rex.layout.block{
      flavor = FLAVOR,
      label = "clump:" .. root,
      options = { cwd = root, command = cmd },
    },
  }
  if not res then error("clump: new_layer: " .. tostring(err), 0) end
  return res
end

-- pick_items lists "this branch vs default" and the recent commits.
local function pick_items(root)
  local items = {}
  local br = rex.exec({ "git", "-C", root, "symbolic-ref", "--quiet", "--short", "HEAD" }, { timeout = 5 })
  local branch = br and br.ok and br.stdout:gsub("%s+$", "") or nil
  if branch and branch ~= "main" and branch ~= "master" then
    items[#items + 1] = { id = "branch", title = "This branch vs default", subtitle = branch, icon = "branch" }
  end
  local r = rex.exec({ "git", "-C", root, "log", "-n", "40", "--format=%H%x09%h%x09%cr%x09%s" }, { timeout = 10 })
  if r and r.ok then
    for line in r.stdout:gmatch("[^\n]+") do
      local sha, short, when, subject = line:match("^(%x+)\t(%x+)\t([^\t]*)\t(.*)$")
      if sha then
        items[#items + 1] = { id = sha, title = short .. " " .. subject, subtitle = when, icon = "tag" }
      end
    end
  end
  return items
end

M.pick_items = pick_items

-- Reasons rex.ui.pick gives when it cannot show a picker at all; clump then
-- shows its own picker in the floating layer.
local NO_PICKER = { unsupported = true, no_client = true, busy = true, timeout = true, failed = true }

rex.action{
  name = "clump_pick",
  title = "clump: Review a commit or this branch",
  description = "Pick a commit (or this branch vs the default branch) and open clump's grouped diff in a floating layer.",
  category = "clump",
  keywords = "diff review group commit branch",
  args = { block = "string?", repo = "string?", fallback = "boolean?" },
  run = function(ctx, args)
    args = args or {}
    local session, block, root = context(ctx, args)
    local id, why = nil, "unsupported"
    if not args.fallback then
      id, why = rex.ui.pick{
        title = "clump: review what?",
        placeholder = "Filter commits",
        items = pick_items(root),
      }
    end
    if id == nil then
      if why == "cancelled" then return { picked = false } end
      if NO_PICKER[why] or args.fallback then
        open_oneshot(session, block, root, { "--pick" })
        return { picked = false, fallback = true, reason = why }
      end
      error("clump: pick: " .. tostring(why), 0)
    end
    local argv = { id }
    if id == "branch" then argv = { "--branch" } end
    open_oneshot(session, block, root, argv)
    return { picked = id }
  end,
}

if not rawget(_G, "CLUMP_NO_BINDINGS") then
  rex.bind("ctrl+alt+g", "clump_pick")
  rex.bind("ctrl+alt+w", "clump_watch")
  rex.bind("ctrl+alt+q", "clump_watch_close")
end

return M
