"""FO3ID 私人服务器的后台管理页：http://127.0.0.1:8088/（仅限本机访问）。

由 http_config.py 在独立线程里启动（环境变量 FO3_ADMIN_PORT，默认 8088，0 = 关闭）。
页面（admin/index.html）调用下面的 JSON 接口，接口通过 http_config 自己的辅助函数读写
gamedata/state.json：EP、球员（添加 / 移除 / 强化等级 / 主力-替补阵容）、礼包、背包礼物、
每日登录奖励表、球探冷却和服务器日志。

防护：只允许 127.0.0.1 / localhost 连接；任何修改都必须是带 X-FO3-Admin 头的 POST
（其他来源的页面无法在不触发本服务从不放行的 CORS 预检的情况下发出该头）。
ponytail: 没有登录；本机任何人都能用，需要的话自己加密码。
"""
import http.server
import json
import os
import threading
import time
import urllib.parse
from contextlib import closing

HERE = os.path.dirname(os.path.abspath(__file__))
PAGE = os.path.join(HERE, "admin", "index.html")
POS_NAMES = "GK SW RWB RB RCB CB LCB LB LWB RDM CDM LDM RM RCM CM LCM LM RAM CAM LAM RF CF LF RW RS ST LS LW".split()
API = {}
READ_ONLY = {"summary", "players", "search", "packs", "daily", "log"}  # 其余接口必须是 POST + 该头
hc = None  # http_config 模块，由 start()（或测试）注入


class AdminError(Exception):
    pass


def api(name):
    def reg(fn):
        API[name] = fn
        return fn
    return reg


def coach(st):
    return hc.my_coach(st)


def team(st):
    return st["teams"][str(hc._default_slot(st))]


def names(spids):
    """{spid: player name} from fifadb."""
    spids = sorted({int(s) for s in spids})
    if not spids:
        return {}
    with closing(hc._db()) as c:
        return {s: n for s, n in c.execute(
            f"""SELECT p.spid, COALESCE(n.full_eng, n.full, n.short) FROM players p JOIN nameseng n ON n.playerid = p.playerid
                WHERE p.spid IN ({','.join('?' * len(spids))})""", spids)}


def player(st, pid):
    p = next((x for x in team(st)["players"] if hc.iid(x) == str(pid)), None)
    if p is None:
        raise AdminError("找不到该球员")
    return p


def as_int(v, lo, hi, what):
    try:
        v = int(v)
    except (TypeError, ValueError):
        raise AdminError(f"{what} 必须是数字")
    if not lo <= v <= hi:
        raise AdminError(f"{what} 必须在 {lo}..{hi} 之间")
    return v


@api("summary")
def summary(p):
    st = hc._state(); c = coach(st); slot = hc._default_slot(st)
    d = c.get("daily") or {"last": "", "streak": 0, "total": 0}
    return {"coach": {"name": c.get("name"), "ep": c.get("ep", hc.START_EP), "teamid": c.get("teamid")},
            "vip_until": time.strftime("%Y-%m-%d", time.gmtime(hc.VIP_UNTIL / 1000)), "igr": True,
            "players": len(team(st)["players"]), "max_players": hc.MAX_PLAYERS,
            "packs": sum(i["remains"] for i in c.get("items", [])), "bag": len(c.get("bag", [])),
            "daily": {**d, "today": hc.today_wib(), "granted_today": d.get("last") == hc.today_wib()},
            "scouts": hc.coach_scouters(st, slot),
            "server_time": time.strftime("%Y-%m-%d %H:%M:%S", time.gmtime(time.time() + hc.WIB)) + " WIB"}


@api("players")
def players(p):
    st = hc._state(); t = team(st)
    pos = {str(s.get("id") or s["spid"]): s["pos"] for s in t["squad"]["positions"]}
    nm = names(x["spid"] for x in t["players"])
    out = []
    for x in t["players"]:
        i, g = hc.iid(x), hc.grade_of(x)
        out.append({"id": i, "spid": x["spid"], "name": nm.get(x["spid"], "?"), "season": x["spid"] // 1000000,
                    "ovr": hc.card_ovr(x["spid"]) + hc.GRADE_BOOST[g], "grade": g, "price": hc.card_price(x["spid"], g),
                    "role": "MAIN" if pos.get(i, 99) < hc.POS_SUB else "SUB" if i in pos else "RES",
                    "team2": bool(x.get("team2"))})
    return {"players": out}


@api("search")
def search(p):
    q = str(p.get("q") or "").strip()
    if len(q) < 2:
        return {"rows": []}
    with closing(hc._db()) as c:
        rows = c.execute(f"""SELECT p.spid, COALESCE(n.full_eng, n.full, n.short), {hc.POSOVR}, p.preferredposition1
                             FROM players p JOIN nameseng n ON n.playerid = p.playerid
                             WHERE n.full LIKE ?1 OR n.short LIKE ?1 OR n.full_eng LIKE ?1 OR n.short_eng LIKE ?1
                             ORDER BY 3 DESC LIMIT 60""", (f"%{q}%",)).fetchall()
    return {"rows": [{"spid": s, "name": n, "season": s // 1000000, "ovr": o, "pos": POS_NAMES[pp] if 0 <= pp < 28 else "?",
                      "market": hc.on_market(s), "price": hc.card_price(s)} for s, n, o, pp in rows]}


@api("packs")
def packs(p):
    return {"packs": [{"itemid": i, "name": n, "price": hc.pack_price(i)} for i, (n, _) in hc.PACKS.items()]}


@api("ep")
def set_ep(p):
    st = hc._state(); c = coach(st)
    if "set" in p:
        c["ep"] = as_int(p["set"], 0, 10 ** 13, "EP")
    else:
        hc.add_ep(st, as_int(p.get("add"), -10 ** 13, 10 ** 13, "EP"))
        c["ep"] = max(0, c["ep"])
    hc._save(st)
    return {"ep": c["ep"]}


@api("player_add")
def player_add(p):
    st = hc._state(); t = team(st)
    spid, grade = as_int(p.get("spid"), 1, 10 ** 9, "spid"), as_int(p.get("grade", 1), 1, 10, "强化等级")
    if not names([spid]):
        raise AdminError("数据库中不存在这个 spid")
    if len(t["players"]) >= hc.MAX_PLAYERS:
        raise AdminError(f"俱乐部已满（{hc.MAX_PLAYERS} 名球员）")
    x = hc.add_player(t, spid, grade)
    hc._save(st)
    return {"id": hc.iid(x)}


@api("player_grade")
def player_grade(p):
    st = hc._state()
    player(st, p.get("id"))["grade"] = as_int(p.get("grade"), 0, 10, "强化等级")
    hc._save(st)
    return {"ok": True}


@api("player_remove")
def player_remove(p):
    st = hc._state(); t = team(st); x = player(st, p.get("id"))
    if hc.in_squad(t, x):
        raise AdminError("该球员在 18 人名单里，请先在阵型界面替换")
    hc.remove_player(t, x)
    hc._save(st)
    return {"ok": True}


@api("player_team")
def player_team(p):
    st = hc._state(); t = team(st); x = player(st, p.get("id"))
    if p.get("team") == "second":
        if hc.in_squad(t, x):
            raise AdminError("18 人名单里的球员仍留在主力阵容")
        x["team2"] = True
    else:
        x.pop("team2", None)
    hc._save(st)
    return {"ok": True}


def give_pack(st, itemid, count):
    inv = coach(st).setdefault("items", [])
    stack = next((i for i in inv if i["itemid"] == itemid), None)
    if stack is None:
        stack = {"_id": hc.new_iid(), "itemid": itemid, "remains": 0}
        inv.append(stack)
    stack["remains"] += count


@api("give_pack")
def api_give_pack(p):
    st = hc._state()
    itemid = as_int(p.get("itemid"), 1, 10 ** 10, "礼包")
    if itemid not in hc.PACKS:
        raise AdminError("未知的礼包")
    give_pack(st, itemid, as_int(p.get("count", 1), 1, 999, "数量"))
    hc._save(st)
    return {"ok": True}


@api("bag_send")
def bag_send(p):
    """背包里的一份礼物（我的道具 > 道具）：EP 和/或礼包和/或球员。"""
    st = hc._state()
    items = hc.reward_items({"ep": as_int(p.get("ep") or 0, 0, 10 ** 12, "EP"),
                             "pack": as_int(p["pack"], 1, 10 ** 10, "礼包") if p.get("pack") else None,
                             "count": as_int(p.get("count") or 1, 1, 999, "数量")})
    if any(i["itemid"] != hc.EP_ITEM and i["itemid"] not in hc.PACKS for i in items):
        raise AdminError("未知的礼包")
    players_ = []
    if p.get("spid"):
        spid = as_int(p["spid"], 1, 10 ** 9, "spid")
        if not names([spid]):
            raise AdminError("数据库中不存在这个 spid")
        players_ = [{"spid": spid, "grade": as_int(p.get("grade") or 1, 1, 10, "强化等级")}]
    if not items and not players_:
        raise AdminError("请填写 EP、礼包或球员")
    e = hc.bag_add(st, str(p.get("title") or "管理员奖励")[:60], items, players_, msg="来自服务器管理员的奖励。")
    hc._save(st)
    return {"id": e["_id"]}


@api("daily")
def daily(p):
    st = hc._state()
    return {"table": hc.daily_table(), "status": coach(st).get("daily") or {"last": "", "streak": 0, "total": 0},
            "today": hc.today_wib(), "file": os.path.relpath(hc.DAILY_FILE, HERE)}


@api("daily_set")
def daily_set(p):
    table = []
    for i, r in enumerate(p.get("table") or []):
        ep = as_int(r.get("ep") or 0, 0, 10 ** 12, f"第 {i + 1} 天的 EP")
        pack = as_int(r["pack"], 1, 10 ** 10, f"第 {i + 1} 天的礼包") if r.get("pack") else None
        if pack is not None and pack not in hc.PACKS:
            raise AdminError(f"第 {i + 1} 天的礼包未知")
        if not ep and pack is None:
            raise AdminError(f"第 {i + 1} 天为空")
        table.append({**({"ep": ep} if ep else {}),
                      **({"pack": pack, "count": as_int(r.get("count") or 1, 1, 99, "数量")} if pack else {})})
    if not 1 <= len(table) <= 31:
        raise AdminError("表格需要 1..31 天")
    with open(hc.DAILY_FILE, "w", encoding="utf-8") as f:
        json.dump(table, f, indent=1)
    return {"table": table}


@api("daily_reset")
def daily_reset(p):
    st = hc._state()
    coach(st)["daily"] = {"last": "", "streak": 0, "total": 0}
    hc._save(st)
    return {"ok": True}


@api("daily_grant")
def daily_grant(p):
    """立刻发放下一天的奖励（相当于明天再登录一次），用于测试。"""
    st = hc._state()
    d = coach(st).setdefault("daily", {"last": "", "streak": 0, "total": 0})
    if d["last"] == hc.today_wib():
        d["last"] = hc.today_wib(time.time() - 86400)  # 让连续登录继续累加
    e = hc.daily_login(st)
    hc._save(st)
    return {"title": e["title"] if e else None}


@api("scouts_reset")
def scouts_reset(p):
    st = hc._state()
    coach(st)["scout_used"] = {}
    hc._save(st)
    return {"ok": True}


@api("log")
def log(p):
    n = as_int(p.get("n", 80), 1, 1000, "n")
    try:
        with open(hc.LOG, encoding="utf-8", errors="replace") as f:
            lines = f.readlines()[-n:]
    except OSError:
        lines = []
    return {"lines": [ln.rstrip("\n")[:300] for ln in lines]}


class Admin(http.server.BaseHTTPRequestHandler):
    def _send(self, code, data, ctype="application/json"):
        self.send_response(code)
        self.send_header("Content-Type", ctype)
        self.send_header("Content-Length", str(len(data)))
        self.send_header("Cache-Control", "no-store")
        self.end_headers()
        self.wfile.write(data)

    def _route(self):
        host = (self.headers.get("Host") or "").rsplit(":", 1)[0]
        if self.client_address[0] not in ("127.0.0.1", "::1") or host not in ("127.0.0.1", "localhost", "[::1]"):
            return self._send(403, b"forbidden", "text/plain")
        url = urllib.parse.urlparse(self.path)
        if url.path in ("/", "/index.html"):
            with open(PAGE, "rb") as f:
                return self._send(200, f.read(), "text/html; charset=utf-8")
        name = url.path[len("/api/"):] if url.path.startswith("/api/") else ""
        fn = API.get(name)
        if fn is None:
            return self._send(404, b'{"error": "not found"}')
        if name not in READ_ONLY and (self.command != "POST" or self.headers.get("X-FO3-Admin") != "1"):
            return self._send(403, b'{"error": "changes need POST with the X-FO3-Admin header"}')
        params = dict(urllib.parse.parse_qsl(url.query))
        n = int(self.headers.get("Content-Length") or 0)
        try:
            if n:
                params.update(json.loads(self.rfile.read(n) or b"{}"))
            with hc.STATE_LOCK:  # 与游戏请求用的是同一把锁
                out = fn(params)
        except AdminError as e:
            out = {"error": str(e)}
        except Exception as e:  # 保持页面可用；具体信息显示在页面的状态栏里
            out = {"error": f"{type(e).__name__}: {e}"}
        self._send(200, json.dumps(out).encode())

    do_GET = _route
    do_POST = _route

    def log_message(self, *a):
        pass


def start(module, port):
    """在守护线程里把后台管理页跑在 127.0.0.1:<port> 上。"""
    global hc
    hc = module
    srv = http.server.ThreadingHTTPServer(("127.0.0.1", port), Admin)
    threading.Thread(target=srv.serve_forever, daemon=True, name="admin").start()
    return srv
