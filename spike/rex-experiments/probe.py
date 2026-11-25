# Probe terminal replies: CSI ? 996 n (color scheme), OSC 7501 ? (program status), DA1.
# Then emit OSC 2 title and OSC 7501 states with pauses so an outside observer can read them.
import os, sys, termios, tty, time, select, json, base64
out = sys.argv[1]
fd = sys.stdin.fileno()
old = termios.tcgetattr(fd)
res = {}
def query(seq, timeout=1.0):
    tty.setraw(fd)
    try:
        os.write(1, seq.encode() + b"\x1b[c")  # DA1 sentinel
        buf = b""; t = time.time()
        while time.time() - t < timeout:
            r, _, _ = select.select([fd], [], [], 0.05)
            if r:
                buf += os.read(fd, 1024)
                # DA1 reply ends with 'c' after CSI ?
                if b"\x1b[?" in buf and buf.rstrip().endswith(b"c") and b"c" in buf.split(b"\x1b[?")[-1]:
                    break
        return buf
    finally:
        termios.tcsetattr(fd, termios.TCSADRAIN, old)
for name, seq in [("csi996", "\x1b[?996n"), ("osc7501", "\x1b]7501;?\x1b\\"), ("osc11_bg", "\x1b]11;?\x1b\\")]:
    t = time.time(); b = query(seq); res[name] = {"reply": b.decode("latin1"), "ms": round((time.time()-t)*1000, 1)}
res["env"] = {k: v for k, v in os.environ.items() if k.startswith("REX_") or k in ("TERM", "TERM_PROGRAM", "COLORTERM", "COLORFGBG")}
json.dump(res, open(out, "w"), indent=1)
def st(**kw):
    body = ":".join(f"{k}={v}" for k, v in kw.items())
    os.write(1, f"\x1b]7501;{body}\x1b\\".encode())
b64 = lambda s: base64.b64encode(s.encode()).decode()
os.write(1, b"\x1b]2;clump title test\x1b\\")
print(json.dumps(res)); sys.stdout.flush()
open(out + ".phase", "w").write("working")
st(state="working", app="clump", msg=b64("Grouping 12 hunks with cheap (deepseek-chat)")); time.sleep(3)
open(out + ".phase", "w").write("blocked")
st(state="blocked", kind="question", app="clump", msg=b64("~48k tokens is over the ceiling: press g to group")); time.sleep(3)
open(out + ".phase", "w").write("done")
st(state="done", app="clump", msg=b64("5 groups")); time.sleep(3)
open(out + ".phase", "w").write("error")
st(state="error", app="clump", msg=b64("provider failed: 401")); time.sleep(3)
open(out + ".phase", "w").write("end")
time.sleep(600)
