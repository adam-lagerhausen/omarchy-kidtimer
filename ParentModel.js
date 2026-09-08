.pragma library

var DAY_MIN = 1440
var MIN_BED = 60
var MAX_FUN_MIN = 480
var SIT_GAP_MIN = 10
var DUST_MIN = 2
var LOG_MAX = 6
var LOG_KEEP = 5
var DAY = ["MON", "TUE", "WED", "THU", "FRI", "SAT", "SUN"]
var CATALOG = [
  { id: "minecraft", name: "Minecraft" },
  { id: "roblox", name: "Roblox" },
  { id: "youtube", name: "YouTube" },
  { id: "steam", name: "Steam" },
  { id: "epic", name: "Epic" },
  { id: "twitch", name: "Twitch" },
  { id: "discord", name: "Discord" },
  { id: "spotify", name: "Spotify" },
  { id: "chrome", name: "Chrome" }
]
var DEFAULT_FUN_IN = {
  minecraft: true,
  roblox: true,
  youtube: true,
  steam: true,
  epic: true,
  twitch: true,
  discord: false,
  spotify: false,
  chrome: false
}
var WEEK_DEFAULT = [3600, 3600, 3600, 3600, 3600, 7200, 7200]
var FIXTURE_NOW = 16 + 42 / 60

function clone(v) {
  return JSON.parse(JSON.stringify(v))
}

function pad2(n) {
  var s = String(n)
  if (s.length < 2) return "0" + s
  return s
}

function remainingFor(groups, id) {
  if (!groups || !id) return 0
  var v = groups[id]
  if (v === undefined || v === null) return 0
  return Number(v) || 0
}

function kidSettingsRow(row, fallbackToken) {
  var k = row || {}
  return {
    name: k.name || "",
    url: k.url || "http://127.0.0.1:8742",
    token: k.token || fallbackToken || ""
  }
}

function kidtimerBin(settings, home, bundled) {
  var s = settings || {}
  if (s.parentBin) return String(s.parentBin)
  if (s.kidBin) return String(s.kidBin)
  var local = home ? String(home) + "/.local/bin/kidtimer" : ""
  if (local && binPresent(local)) return local
  if (bundled) return String(bundled)
  if (local) return local
  return "kidtimer"
}

function binPresent(path) {
  if (typeof kidtimerBinExists === "function") return kidtimerBinExists(path) === true
  return true
}

function parentBin(settings, home) {
  return kidtimerBin(settings, home, "")
}

function validPin(digits) {
  var s = String(digits || "")
  if (s.length !== 4) return false
  for (var i = 0; i < 4; i++) {
    var c = s.charAt(i)
    if (c < "0" || c > "9") return false
  }
  return true
}

function pinBoxText(digits, index) {
  var s = String(digits || "")
  if (index < 0 || index >= 4) return ""
  return index < s.length ? s.charAt(index) : ""
}

function parentPinLabel() {
  return "Parent Pin"
}

function parentPinWhy() {
  return "Required for the controls. Use it to make changes on the kids computer."
}

function pinSlotKind(digits, index, caret, committed) {
  if (committed) return "dot"
  var s = String(digits || "")
  if (index < 0 || index >= 4) return "empty"
  if (index < s.length) return "digit"
  if (caret === index && caret < 4) return "caret"
  return "empty"
}

function householdPinArmed(snapshots, householdPin) {
  if (householdPin) return true
  var list = snapshots || []
  for (var i = 0; i < list.length; i++) {
    var st = snapshotStatus(list[i])
    if (st && st.parentPinSet) return true
  }
  return false
}

function kidsFromSettings(settings) {
  var s = settings || {}
  var fallback = s.parentToken || ""
  if (s.kids && s.kids.length) {
    var out = []
    for (var i = 0; i < s.kids.length; i++) {
      out.push(kidSettingsRow(s.kids[i], fallback))
    }
    return out
  }
  return [kidSettingsRow({
    name: s.kidName || "",
    url: s.url || "http://127.0.0.1:8742",
    token: s.parentToken || ""
  }, fallback)]
}

function parseHousehold(raw) {
  if (!raw) return []
  var doc = raw
  if (typeof raw === "string") {
    try {
      doc = JSON.parse(raw)
    } catch (e) {
      return []
    }
  }
  var out = []
  var seenKey = {}
  function add(row, claimed) {
    if (!row || (!row.id && !row.url)) return
    var k = row.id || row.url || ""
    if (k && seenKey[k]) return
    if (k) seenKey[k] = true
    var isClaimed = claimed === true || row.claimed === true
    out.push({
      id: row.id || "",
      name: row.name || "",
      url: row.url || "",
      token: isClaimed ? "" : (row.token || ""),
      claimed: isClaimed,
      live: row.live === true,
      status: row.status || {},
      asks: row.asks || [],
      look: row.look || null,
      reachable: isClaimed || row.live === true || row.reachable === true
    })
  }
  var list = doc && doc.kids ? doc.kids : []
  for (var i = 0; i < list.length; i++) add(list[i], false)
  var seen = doc && doc.seen ? doc.seen : []
  for (var j = 0; j < seen.length; j++) add(seen[j], true)
  return out
}

function mergeKids(settings, discovered) {
  var s = settings || {}
  if (s.lab !== true) return discovered || []
  var pins = []
  if (s.kids && s.kids.length) {
    pins = kidsFromSettings(s)
  } else if (!discovered || !discovered.length) {
    pins = kidsFromSettings(s)
  }
  var extra = discovered || []
  var out = []
  var seen = {}
  function add(row) {
    var k = row.id || row.url || ""
    if (k && seen[k]) return
    if (k) seen[k] = true
    out.push({
      id: row.id || "",
      name: row.name || "",
      url: row.url || "http://127.0.0.1:8742",
      token: row.claimed ? "" : (row.token || s.parentToken || ""),
      claimed: !!row.claimed
    })
  }
  for (var i = 0; i < pins.length; i++) add(pins[i])
  for (var j = 0; j < extra.length; j++) add(extra[j])
  return out
}

function askId(ask) {
  if (!ask) return ""
  return String(ask.id || "")
}

function pendingList(payload) {
  if (!payload) return []
  if (payload.asks) return payload.asks
  if (payload.length) return payload
  return []
}

function pendingIds(payload) {
  var list = pendingList(payload)
  var out = []
  for (var i = 0; i < list.length; i++) {
    var id = askId(list[i])
    if (id) out.push(id)
  }
  return out
}

function newAskIds(previousIds, payload) {
  var seen = {}
  var prev = previousIds || []
  for (var i = 0; i < prev.length; i++) seen[String(prev[i])] = true
  var incoming = pendingIds(payload)
  var fresh = []
  for (var j = 0; j < incoming.length; j++) {
    if (!seen[incoming[j]]) fresh.push(incoming[j])
  }
  return fresh
}

function findAsk(payload, id) {
  var list = pendingList(payload)
  for (var i = 0; i < list.length; i++) {
    if (askId(list[i]) === String(id)) return list[i]
  }
  return null
}

function defaultGroup(groups) {
  return "fun"
}

function grantPayload(group, seconds, groups) {
  var g = "fun"
  var s = Number(seconds)
  if (!isFinite(s) || s === 0) s = 600
  var mins = Math.round(Math.abs(s) / 60)
  return { group: g, seconds: s, reason: (s < 0 ? "-" : "+") + mins }
}

function lockPayload(locked) {
  return { locked: !!locked }
}

function bedtimePayload(start, end) {
  if (typeof start === "number") start = hhmmFromMin(start)
  if (typeof end === "number") end = hhmmFromMin(end)
  return { bedtime_start: String(start || ""), bedtime_end: String(end || "") }
}

function hostSafe(s, max) {
  var n = max || 80
  var out = String(s || "").replace(/[<>&]/g, "")
  if (out.length > n) out = out.slice(0, n)
  return out
}

function householdBarLabel(snapshots) {
  var list = snapshots || []
  if (!list.length) return "no computers"
  var parts = []
  for (var i = 0; i < list.length; i++) {
    parts.push(hostSafe((list[i] && list[i].name) || "kid", 40))
  }
  return hostSafe(parts.join(" · "), 80)
}

function notifyHeadline(kidName) {
  return hostSafe(kidName || "kid", 40)
}

function notifySummary(ask, look, status) {
  if (!ask) return "new ask"
  var m = Math.max(0, Math.floor(askSeconds(ask) / 60))
  return "+" + m + "m"
}

function notifyBody(kidName, ask, look, status) {
  return notifyHeadline(kidName) + " wants " + notifySummary(ask, look, status)
}

function askSeconds(ask) {
  if (!ask) return 0
  var s = ask.seconds
  if (s === undefined || s === null) s = ask["seconds"]
  return Number(s) || 0
}

function askCardText(kidName, seconds) {
  var m = Math.max(0, Math.round(Number(seconds) / 60))
  if (!m) m = 10
  return kidName + " asked for " + m + " more minutes"
}

function lockLabel(name, locked) {
  return (locked ? "Unlock " : "Lock ") + (name || "kid")
}

function formatMinutes(seconds) {
  var n = Math.floor(Number(seconds) / 60)
  if (isNaN(n) || n < 0) n = 0
  return minutesLabel(n)
}

function minutesLabel(n) {
  n = Math.max(0, Math.round(Number(n) || 0))
  if (n >= 60) {
    var h = Math.floor(n / 60)
    var m = n % 60
    return m ? h + "h " + m + "m" : h + "h"
  }
  return n + "m"
}

function minFromHHMM(hhmm) {
  var p = String(hhmm || "00:00").split(":")
  var h = Number(p[0]) || 0
  var m = Number(p[1]) || 0
  return ((h * 60 + m) % DAY_MIN + DAY_MIN) % DAY_MIN
}

function hhmmFromMin(min) {
  var t = ((Math.round(Number(min)) % DAY_MIN) + DAY_MIN) % DAY_MIN
  return pad2(Math.floor(t / 60)) + ":" + pad2(t % 60)
}

function hour12On(v) {
  return v !== false
}

function clockPretty(min) {
  return clockLabel(min, true)
}

function clock24(min) {
  return clockLabel(min, false)
}

function clockLabel(min, hour12) {
  min = ((Math.round(min) % DAY_MIN) + DAY_MIN) % DAY_MIN
  var h = Math.floor(min / 60)
  var m = min % 60
  if (!hour12On(hour12)) return pad2(h) + ":" + pad2(m)
  var ap = h >= 12 ? "PM" : "AM"
  var hr = ((h + 11) % 12) + 1
  return hr + ":" + pad2(m) + " " + ap
}

function trackHours(hour12) {
  if (hour12On(hour12)) return ["12a", "6a", "12p", "6p", "12a"]
  return ["0", "6", "12", "18", "24"]
}

function emptyTrack(hour12) {
  return { beds: [], blocks: [], needle: null, log: [], hours: trackHours(hour12) }
}

function parsePrefs(raw) {
  var doc = raw
  if (typeof raw === "string") {
    try {
      doc = JSON.parse(raw)
    } catch (e) {
      return { hour12: true }
    }
  }
  if (!doc || typeof doc !== "object") return { hour12: true }
  return { hour12: doc.hour12 !== false }
}

function prefsWire(hour12) {
  return JSON.stringify({ hour12: hour12On(hour12) })
}

function hour12Payload(on) {
  return { hour12: hour12On(on) }
}

function clockFromHour(hour) {
  return clock24(Math.round(Number(hour) * 60))
}

function weekdayMondayFirst(now) {
  var d = now && typeof now.getDay === "function" ? now : new Date()
  return (d.getDay() + 6) % 7
}

function chromeHome() {
  return { face: "home", picker: false, bell: false, adopt: null, query: { fun: "", school: "" }, hits: { fun: [], school: [] } }
}

function chromeSettings() {
  return { face: "settings", picker: false, bell: false, adopt: null, query: { fun: "", school: "" }, hits: { fun: [], school: [] } }
}

function emptyLookRaw() {
  return {
    version: 1,
    piles: [{ id: "fun", name: "Fun" }],
    apps: {},
    pile_hours: { fun: 3600 },
    modes: [{ id: "freetime", name: "Freetime", kind: "freetime", hours: {} }],
    sticky_freetime: false,
    schedule: { mon: [], tue: [], wed: [], thu: [], fri: [], sat: [], sun: [] },
    bedtime: { lights_out: 21 * 60, duration: 10 * 60 }
  }
}

function defaultCatalog(funIn) {
  var src = funIn || DEFAULT_FUN_IN
  var out = []
  for (var i = 0; i < CATALOG.length; i++) {
    var id = CATALOG[i].id
    out.push({
      id: id,
      name: CATALOG[i].name,
      inFun: src[id] === true
    })
  }
  return out
}

function defaultPolicy() {
  return {
    bed: 21 * 60,
    up: 7 * 60,
    funDay: WEEK_DEFAULT.slice(),
    catalog: [],
    fun: [],
    school: []
  }
}

function sevenFunDay(raw) {
  if (!raw) return null
  var keys = ["mon", "tue", "wed", "thu", "fri", "sat", "sun"]
  var out = []
  var hit = 0
  for (var i = 0; i < keys.length; i++) {
    var v = raw[keys[i]]
    if (v === undefined || v === null) out.push(WEEK_DEFAULT[i])
    else {
      out.push(Number(v) || 0)
      hit++
    }
  }
  return hit ? out : null
}

function parseAsks(raw) {
  var list = pendingList(raw)
  var out = []
  for (var i = 0; i < list.length; i++) {
    out.push({
      id: askId(list[i]),
      seconds: askSeconds(list[i])
    })
  }
  return out
}

function minutesFromUnix(unix) {
  var n = Number(unix)
  if (!isFinite(n) || n <= 0) return null
  var d = new Date(n * 1000)
  if (isNaN(d.getTime())) return null
  return d.getHours() * 60 + d.getMinutes()
}

function parseSessions(list) {
  var src = list || []
  var out = []
  for (var i = 0; i < src.length; i++) {
    var b = src[i] || {}
    var kind = b.kind || "on"
    if (kind !== "on" && kind !== "free") kind = "on"
    var start
    var fromUnix = minutesFromUnix(b.start_unix != null ? b.start_unix : b.startUnix)
    if (fromUnix != null) start = fromUnix
    else {
      start = b.start
      if (typeof start === "number" && start >= 0 && start <= 24) start = Math.round(start * 60)
      else start = Number(start) || 0
    }
    var dur = b.dur
    if (dur === undefined || dur === null) dur = b.duration
    dur = Number(dur) || 0
    if (dur > 0 && dur < 24 && dur !== Math.floor(dur)) dur = dur * 60
    dur = Math.round(dur)
    out.push({
      kind: kind,
      start: start,
      dur: dur,
      label: String(b.label || kind)
    })
  }
  return out
}

function pickSpent(raw, key) {
  if (!raw) return null
  if (raw.spent && raw.spent[key] != null) return Number(raw.spent[key]) || 0
  if (raw.used && raw.used[key] != null) return Number(raw.used[key]) || 0
  var camel = key === "fun" ? "fun_used_minutes" : "school_used_minutes"
  if (raw[camel] != null) return (Number(raw[camel]) || 0) * 60
  return null
}

function parseStatus(raw) {
  var s = raw || {}
  var focused = s.focused_app ? String(s.focused_app).toLowerCase() : ""
  return {
    parentLocked: !!s.parent_locked,
    parentPinSet: !!s.parent_pin_set || !!s.parentPinSet,
    bedtimeActive: !!s.bedtime_active,
    bedtimeStart: s.bedtime_start ? minFromHHMM(s.bedtime_start) : null,
    bedtimeEnd: s.bedtime_end ? minFromHHMM(s.bedtime_end) : null,
    focusedApp: focused,
    funLeft: remainingFor(s.groups, "fun"),
    spentFun: pickSpent(s, "fun"),
    spentSchool: pickSpent(s, "school"),
    today: parseSessions(s.today || s.sessions || [])
  }
}

function inFunOf(apps, id) {
  if (!apps) return false
  var pile = apps[id]
  if (!pile) return false
  return pile === "fun" || pile === "minecraft" || pile === "youtube"
}

function listedThings(raw, list) {
  var things = raw && raw.things ? raw.things : []
  var apps = raw && raw.apps ? raw.apps : {}
  var out = []
  for (var i = 0; i < things.length; i++) {
    var t = things[i]
    if (!t || !t.id) continue
    if (apps[t.id] === list) out.push({ id: t.id, name: t.name || t.id, kind: t.kind || "app" })
  }
  return out
}

function parseLook(raw) {
  if (!raw || typeof raw !== "object") {
    return { policy: defaultPolicy(), raw: emptyLookRaw() }
  }
  var funDay = sevenFunDay(raw.fun_hours)
  if (!funDay) {
    var one = raw.pile_hours && raw.pile_hours.fun
    if (one != null) {
      funDay = []
      for (var d = 0; d < 7; d++) funDay.push(Number(one) || 0)
    } else funDay = WEEK_DEFAULT.slice()
  }
  var bed = raw.bedtime || {}
  var lights = Number(bed.lights_out)
  if (!isFinite(lights)) lights = 21 * 60
  var dur = Number(bed.duration)
  if (!isFinite(dur) || dur < MIN_BED) dur = 10 * 60
  return {
    policy: {
      bed: lights,
      up: (lights + dur) % DAY_MIN,
      funDay: funDay,
      catalog: [],
      fun: listedThings(raw, "fun"),
      school: listedThings(raw, "school")
    },
    raw: clone(raw)
  }
}

function asStatus(s) {
  if (!s) return parseStatus({})
  if (s.funLeft !== undefined || s.parentLocked !== undefined) return s
  return parseStatus(s)
}

function hostFace(snap) {
  if (snap && snap.claimed) return faceOf("claimed")
  var s = asStatus(snap && snap.status)
  var reachable = !snap || snap.reachable !== false
  if (snap && snap.error) return faceOf("error")
  if (!reachable) return faceOf("offline")
  if (s.parentLocked) return faceOf("locked")
  if (s.bedtimeActive) return faceOf("bedtime")
  if (s.focusedApp) return faceOf("active", s.focusedApp)
  return faceOf("active")
}

function faceOf(kind, app) {
  var caption = ""
  var live = false
  var coral = false
  if (kind === "claimed") {
    caption = "Already claimed"
  } else if (kind === "error") {
    caption = "Error"
    coral = true
  } else if (kind === "offline") {
    caption = "Offline"
  } else if (kind === "locked") {
    caption = "Locked"
    coral = true
  } else if (kind === "bedtime") {
    caption = "Bedtime"
  } else if (kind === "active") {
    caption = app ? "Active · " + app : "Active"
    live = true
  }
  return {
    kind: kind,
    caption: caption,
    live: live,
    coral: coral,
    led: live,
    doing: app || "",
    app: app || ""
  }
}

function computerCaption(face) {
  if (!face) return ""
  if (typeof face === "string") return face
  return face.caption || ""
}

function fillPct(leftMin, usedMin, allotMin) {
  var left = Math.max(0, Number(leftMin) || 0)
  if (usedMin == null) return Math.min(100, Math.round((left / Math.max(1, Number(allotMin) || 1)) * 100))
  return Math.min(100, Math.round((left / Math.max(1, Number(usedMin) + left)) * 100))
}

function projectFun(status, allotSec) {
  var left = Number(status && status.funLeft) || 0
  var used = status ? status.spentFun : null
  var leftMin = Math.floor(left / 60)
  var usedMin = used == null ? null : Math.floor(used / 60)
  var allotMin = Math.floor((Number(allotSec) || 0) / 60)
  return {
    usedSec: used,
    leftSec: left,
    allotSec: Number(allotSec) || 0,
    usedLabel: minutesLabel(usedMin == null ? 0 : usedMin),
    leftLabel: minutesLabel(leftMin) + " LEFT",
    fillPct: fillPct(leftMin, usedMin, allotMin),
    empty: leftMin === 0,
    barLow: leftMin <= 8,
    guessedUsed: usedMin == null
  }
}

function bedSpans(bed, up) {
  var start = ((Number(bed) % DAY_MIN) + DAY_MIN) % DAY_MIN
  var end = ((Number(up) % DAY_MIN) + DAY_MIN) % DAY_MIN
  if (end > start) return [{ start: start, end: end }]
  return [{ start: 0, end: end }, { start: start, end: DAY_MIN }]
}

function spanPct(span) {
  return {
    leftPct: (span.start / DAY_MIN) * 100,
    widthPct: ((span.end - span.start) / DAY_MIN) * 100
  }
}

function spentAsSession(status, nowHour) {
  var sec = status && status.spentFun
  if (sec == null || !(Number(sec) > 0)) return []
  var dur = Math.floor(Number(sec) / 60)
  if (dur < 1) dur = 1
  var nowMin = Math.round(Number(nowHour) * 60)
  if (!isFinite(nowMin)) nowMin = 0
  nowMin = ((nowMin % DAY_MIN) + DAY_MIN) % DAY_MIN
  var start = nowMin - dur
  if (start < 0) {
    dur = Math.min(dur, nowMin > 0 ? nowMin : dur)
    start = 0
  }
  return [{
    kind: "on",
    start: start,
    dur: dur,
    label: String((status && status.focusedApp) || "on")
  }]
}

function activitySessions(status, nowHour) {
  var src = (status && status.today) || []
  if (src.length) return src
  return spentAsSession(status, nowHour)
}

function friendlyApp(label) {
  var s = String(label || "").trim()
  if (!s) return "Computer"
  var k = s.toLowerCase()
  if (k === "on" || k === "free") return "Computer"
  if (k === "foot" || k === "footclient" || k.indexOf("dnkl.foot") >= 0) return "Terminal"
  if (k.indexOf("youtube") >= 0) return "YouTube"
  if (k.indexOf("khan") >= 0) return "Khan Academy"
  if (k.indexOf("minecraft") >= 0 || k.indexOf("prism") >= 0) return "Minecraft"
  if (k.indexOf("roblox") >= 0) return "Roblox"
  if (k.indexOf("discord") >= 0) return "Discord"
  if (k.indexOf("spotify") >= 0) return "Spotify"
  if (k.indexOf("twitch") >= 0) return "Twitch"
  if (k.indexOf("steam") >= 0) return "Steam"
  if (k.indexOf("epic") >= 0 || k.indexOf("unreal") >= 0) return "Epic"
  if (k === "chromium" || k.indexOf("chromium") >= 0) return "Chromium"
  if (k === "chrome" || k.indexOf("google-chrome") >= 0 || k.indexOf("chrome") >= 0) return "Chrome"
  return s
}

function sortedSpans(sessions) {
  var src = sessions || []
  var out = []
  for (var i = 0; i < src.length; i++) {
    var s = src[i]
    var start = Number(s && s.start) || 0
    var dur = Math.max(0, Math.round(Number(s && s.dur) || 0))
    if (dur < 1) continue
    out.push({
      kind: (s && s.kind) || "on",
      start: start,
      dur: dur,
      end: start + dur,
      label: String((s && s.label) || (s && s.kind) || "on")
    })
  }
  out.sort(function (a, b) {
    if (a.start !== b.start) return a.start - b.start
    return a.end - b.end
  })
  return out
}

function occupancyBlocks(sessions, nowMin) {
  var src = sortedSpans(sessions)
  var cap = nowMin == null ? null : Math.round(Number(nowMin))
  if (!isFinite(cap)) cap = null
  var merged = []
  for (var i = 0; i < src.length; i++) {
    var s = src[i]
    var last = merged.length ? merged[merged.length - 1] : null
    if (last && s.start <= last.end) {
      if (s.end > last.end) last.end = s.end
      continue
    }
    merged.push({ kind: s.kind, start: s.start, end: s.end, label: s.label })
  }
  var blocks = []
  for (var j = 0; j < merged.length; j++) {
    var b = merged[j]
    var start = b.start
    var end = b.end
    if (cap != null) {
      if (start > cap) continue
      if (end > cap) end = cap
      if (end < start) continue
    }
    var dur = end - start
    if (dur < 0) continue
    if (dur < 1 && (cap == null || start !== cap)) continue
    var widthPct = (dur / DAY_MIN) * 100
    blocks.push({
      kind: b.kind,
      leftPct: (start / DAY_MIN) * 100,
      widthPct: widthPct,
      label: b.label,
      named: b.kind === "on" && widthPct >= 4
    })
  }
  return blocks
}

function sittingName(apps) {
  var rows = []
  for (var name in apps) {
    if (!Object.prototype.hasOwnProperty.call(apps, name)) continue
    rows.push({ name: name, dur: apps[name] })
  }
  rows.sort(function (a, b) {
    if (b.dur !== a.dur) return b.dur - a.dur
    return a.name < b.name ? -1 : a.name > b.name ? 1 : 0
  })
  if (!rows.length) return "COMPUTER"
  var named = []
  for (var i = 0; i < rows.length; i++) {
    if (rows[i].dur >= DUST_MIN) named.push(rows[i].name)
  }
  if (!named.length) named.push(rows[0].name)
  if (named.length > 2) named = named.slice(0, 2)
  return named.join(" + ").toUpperCase()
}

function sittingsFrom(sessions, hour12) {
  var src = sortedSpans(sessions)
  var sits = []
  for (var i = 0; i < src.length; i++) {
    var s = src[i]
    var last = sits.length ? sits[sits.length - 1] : null
    var name = friendlyApp(s.label)
    if (last && s.start <= last.end + SIT_GAP_MIN) {
      if (s.end > last.end) last.end = s.end
      last.apps[name] = (last.apps[name] || 0) + s.dur
      continue
    }
    var apps = {}
    apps[name] = s.dur
    sits.push({ start: s.start, end: s.end, apps: apps })
  }
  var out = []
  for (var j = 0; j < sits.length; j++) {
    var sit = sits[j]
    out.push({
      start: sit.start,
      dur: sit.end - sit.start,
      clock: clockLabel(sit.start, hour12),
      name: sittingName(sit.apps),
      durLabel: minutesLabel(sit.end - sit.start)
    })
  }
  return out
}

function sittingLog(sessions, hour12) {
  var sits = sittingsFrom(sessions, hour12)
  if (sits.length <= LOG_MAX) {
    var all = []
    for (var i = 0; i < sits.length; i++) {
      all.push({ clock: sits[i].clock, name: sits[i].name, dur: sits[i].durLabel })
    }
    return all
  }
  var fold = sits.slice(0, sits.length - LOG_KEEP)
  var rest = sits.slice(sits.length - LOG_KEEP)
  var earlierDur = 0
  for (var f = 0; f < fold.length; f++) earlierDur += fold[f].dur
  var log = [{
    clock: fold[0].clock,
    name: "EARLIER",
    dur: minutesLabel(earlierDur)
  }]
  for (var r = 0; r < rest.length; r++) {
    log.push({ clock: rest[r].clock, name: rest[r].name, dur: rest[r].durLabel })
  }
  return log
}

function layoutTrack(sessions, policy, nowHour, withToday, hour12) {
  var beds = bedSpans(policy.bed, policy.up)
  var bedPct = []
  for (var i = 0; i < beds.length; i++) bedPct.push(spanPct(beds[i]))
  var blocks = []
  var log = []
  var nowMin = nowHour == null ? null : Math.round(Number(nowHour) * 60)
  if (withToday) {
    blocks = occupancyBlocks(sessions, nowMin)
    log = sittingLog(sessions, hour12)
  }
  var needle = null
  if (withToday && nowHour != null) needle = (Number(nowHour) / 24) * 100
  return {
    beds: bedPct,
    blocks: blocks,
    needle: needle,
    log: log,
    hours: trackHours(hour12)
  }
}

function snapshotStatus(snap) {
  return asStatus(snap && snap.status)
}

function snapshotLook(snap) {
  var l = snap && snap.look
  if (!l) return parseLook(null)
  if (l.policy) return l
  return parseLook(l)
}

function snapshotAsks(snap) {
  var a = snap && snap.asks
  if (!a) return []
  if (a.length && a[0] && a[0].seconds !== undefined && a[0].id !== undefined && !a.asks) {
    if (a[0].kidName) return a
    return a
  }
  return parseAsks(a)
}

function projectKid(snap, index, now, allotOverride, hour12) {
  var status = snapshotStatus(snap)
  var look = snapshotLook(snap)
  var policy = look.policy || defaultPolicy()
  if (status.bedtimeStart != null) policy = {
    bed: status.bedtimeStart,
    up: status.bedtimeEnd != null ? status.bedtimeEnd : policy.up,
    funDay: policy.funDay,
    catalog: policy.catalog || [],
    fun: policy.fun || [],
    school: policy.school || []
  }
  var reachable = !snap || snap.reachable !== false
  var claimed = !!(snap && snap.claimed)
  var faceSnap = { reachable: reachable, error: !!(snap && snap.error), claimed: claimed, status: status }
  var face = hostFace(faceSnap)
  var today = weekdayMondayFirst(now)
  var allot = allotOverride != null ? allotOverride : policy.funDay[today]
  var schoolSec = status.spentSchool
  return {
    index: index,
    claimed: claimed,
    name: (snap && snap.name) || "kid",
    nameUp: String((snap && snap.name) || "kid").toUpperCase(),
    face: face,
    locked: !!status.parentLocked,
    schoolLabel: minutesLabel(schoolSec == null ? 0 : Math.floor(schoolSec / 60)),
    fun: projectFun(status, allot),
    policy: {
      bed: policy.bed,
      up: policy.up,
      bedLabel: clockLabel(policy.bed, hour12),
      upLabel: clockLabel(policy.up, hour12),
      funDay: policy.funDay.slice(),
      funDayRows: (function () {
        var rows = []
        for (var r = 0; r < 7; r++) {
          rows.push({
            day: r,
            dow: DAY[r],
            label: minutesLabel(Math.floor((Number(policy.funDay[r]) || 0) / 60))
          })
        }
        return rows
      })(),
      catalog: policy.catalog || [],
      fun: policy.fun || [],
      school: policy.school || []
    },
    pickerLine: (face.live ? "●" : "○") + " " + face.caption + "  " + String((snap && snap.name) || "kid").toUpperCase(),
    on: face.caption
  }
}

function householdAsks(snapshots) {
  var list = snapshots || []
  var out = []
  for (var i = 0; i < list.length; i++) {
    if (list[i] && list[i].claimed) continue
    var name = (list[i] && list[i].name) || "kid"
    var asks = snapshotAsks(list[i])
    for (var j = 0; j < asks.length; j++) {
      var a = asks[j]
      out.push({
        id: a.id,
        kidIndex: i,
        kidName: name,
        seconds: a.seconds,
        text: askCardText(name, a.seconds)
      })
    }
  }
  return out
}

function hourFromNow(now) {
  if (typeof now === "number") return now
  var d = now && typeof now.getHours === "function" ? now : new Date()
  return d.getHours() + d.getMinutes() / 60
}

function emptyHowTo() {
  return {
    title: "No computers found",
    lines: [
      { text: "Run this on the kid computer. It will show up here." },
      { cmd: true, text: "omarchy plugin add https://github.com/adam-lagerhausen/omarchy-kidtimer.git --enable" }
    ]
  }
}

function blankKid() {
  return {
    name: "",
    nameUp: "",
    locked: false,
    face: { live: false, caption: "", coral: false },
    fun: { usedLabel: "0m", leftLabel: "0m LEFT", fillPct: 0, empty: true, barLow: true },
    policy: { bedLabel: "", upLabel: "", funDayRows: [] }
  }
}

function emptyChrome(chrome) {
  var ch = chrome || chromeHome()
  return {
    face: ch.face,
    picker: !!ch.picker,
    bell: !!ch.bell,
    adopt: ch.adopt || null,
    query: ch.query || { fun: "", school: "" },
    hits: ch.hits || { fun: [], school: [] }
  }
}

function adoptPrompt(kid) {
  var name = String((kid && (kid.nameUp || kid.name)) || "this computer").toUpperCase()
  return {
    index: kid && kid.index,
    name: (kid && kid.name) || "",
    title: "Take over " + name + "?",
    body: "Another parent already claimed this computer. Yes makes it yours."
  }
}

function setupPinTape() {
  return {
    waiting: false,
    needsPin: true,
    howTo: { title: "", lines: [] },
    chrome: {
      face: "home",
      picker: false,
      bell: false,
      adopt: null,
      query: { fun: "", school: "" },
      hits: { fun: [], school: [] }
    },
    kid: blankKid(),
    kids: [],
    ours: false,
    asks: [],
    bellCount: 0,
    track: emptyTrack(true),
    showLock: false,
    showStamp: false,
    lockLabel: "Lock",
    pinSet: false,
    lockArmed: false,
    hour12: true
  }
}

function emptyWaitingTape(chrome) {
  return {
    waiting: true,
    needsPin: false,
    howTo: emptyHowTo(),
    chrome: emptyChrome(chrome),
    kid: blankKid(),
    kids: [],
    ours: false,
    adopt: null,
    asks: [],
    bellCount: 0,
    track: emptyTrack(true),
    showLock: false,
    showStamp: false,
    lockLabel: "Lock",
    pinSet: true,
    lockArmed: true,
    hour12: true
  }
}

function projectTape(snapshots, selectedIndex, chrome, now, household) {
  var list = snapshots || []
  var ch = chrome || chromeHome()
  var pinSet = householdPinArmed(list, !!(household && household.pinSet))
  var hour12 = hour12On(household && household.hour12)
  if (!pinSet) return setupPinTape()
  if (!list.length) return emptyWaitingTape(ch)
  var i = Number(selectedIndex) || 0
  if (i < 0 || i >= list.length) i = 0
  var snap = list[i]
  var kid = projectKid(snap, i, now, null, hour12)
  var kids = []
  for (var k = 0; k < list.length; k++) kids.push(projectKid(list[k], k, now, null, hour12))
  var asks = householdAsks(list)
  var status = snapshotStatus(snap)
  var nowHour = hourFromNow(now)
  var withToday = ch.face !== "settings"
  var track = layoutTrack(activitySessions(status, nowHour), kid.policy, nowHour, withToday, hour12)
  var ours = !kid.claimed
  var adopt = ch.adopt ? adoptPrompt(kids[ch.adopt.index] || kid) : null
  return {
    waiting: false,
    needsPin: false,
    howTo: { title: "", lines: [] },
    chrome: {
      face: ch.face,
      picker: !!ch.picker,
      bell: !!ch.bell,
      adopt: ch.adopt || null,
      query: ch.query || { fun: "", school: "" },
      hits: ch.hits || { fun: [], school: [] }
    },
    kid: kid,
    kids: kids,
    ours: ours,
    adopt: adopt,
    asks: ours ? asks : [],
    bellCount: ours ? asks.length : 0,
    track: ours ? track : emptyTrack(hour12),
    showLock: ch.face === "home" && ours,
    showStamp: ch.face === "home" && ours && kid.locked,
    lockLabel: lockLabel(kid.name, kid.locked),
    pinSet: pinSet,
    lockArmed: pinSet && ours,
    hour12: hour12
  }
}

function reduceChrome(chrome, act, tape) {
  var ch = {
    face: (chrome && chrome.face) || "home",
    picker: !!(chrome && chrome.picker),
    bell: !!(chrome && chrome.bell),
    adopt: chrome && chrome.adopt ? chrome.adopt : null,
    query: (chrome && chrome.query) ? chrome.query : { fun: "", school: "" },
    hits: (chrome && chrome.hits) ? chrome.hits : { fun: [], school: [] }
  }
  var kind = act && act.kind
  var out = { chrome: ch }
  if (kind === "bell") {
    if (tape && tape.bellCount > 0) ch.bell = !ch.bell
  } else if (kind === "settings") {
    ch.face = ch.face === "settings" ? "home" : "settings"
    ch.picker = false
    ch.adopt = null
  } else if (kind === "pick") {
    ch.picker = !ch.picker
    ch.adopt = null
  } else if (kind === "select") {
    ch.picker = false
    out.selectIndex = act.kidIndex
    var row = tape && tape.kids && tape.kids[act.kidIndex]
    if (row && row.claimed) ch.adopt = { index: act.kidIndex }
    else ch.adopt = null
  } else if (kind === "adoptNo") {
    ch.adopt = null
  } else if (kind === "adoptYes") {
    out.adopt = ch.adopt
    ch.adopt = null
  } else if (kind === "query") {
    var list = act.list === "school" ? "school" : "fun"
    ch.query = { fun: ch.query.fun, school: ch.query.school }
    ch.query[list] = act.q || ""
    ch.hits = { fun: ch.hits.fun, school: ch.hits.school }
    ch.hits[list] = []
  } else if (kind === "hits") {
    var hitList = act.list === "school" ? "school" : "fun"
    ch.hits = { fun: ch.hits.fun, school: ch.hits.school }
    ch.hits[hitList] = act.hits || []
  } else if (kind === "addThing") {
    var addList = act.list === "school" ? "school" : "fun"
    ch.query = { fun: ch.query.fun, school: ch.query.school }
    ch.query[addList] = ""
    ch.hits = { fun: ch.hits.fun, school: ch.hits.school }
    ch.hits[addList] = []
  }
  return out
}

function nightLen(bed, up) {
  var d = up - bed
  if (d <= 0) d += DAY_MIN
  return d
}

function stepClock(min, delta) {
  return ((Math.round(Number(min)) + Number(delta) + DAY_MIN) % DAY_MIN)
}

function applyPolicy(snapshot, act) {
  var look = clone(snapshotLook(snapshot))
  var p = look.policy || defaultPolicy()
  look.policy = p
  var kind = act && act.kind
  if (kind === "bed" || kind === "up") {
    var status = snapshotStatus(snapshot)
    if (status.bedtimeStart != null) p.bed = status.bedtimeStart
    if (status.bedtimeEnd != null) p.up = status.bedtimeEnd
    if (kind === "bed") p.bed = stepClock(p.bed, act.delta)
    else p.up = stepClock(p.up, act.delta)
    if (nightLen(p.bed, p.up) < MIN_BED) {
      if (kind === "bed") p.up = (p.bed + MIN_BED) % DAY_MIN
      else p.bed = (p.up - MIN_BED + DAY_MIN) % DAY_MIN
    }
    p.bedLabel = clockPretty(p.bed)
    p.upLabel = clockPretty(p.up)
  } else if (kind === "funDay") {
    var day = Number(act.day) || 0
    var next = Math.max(0, Math.min(MAX_FUN_MIN * 60, (Number(p.funDay[day]) || 0) + Number(act.delta) * 60))
    p.funDay[day] = next
  } else if (kind === "addThing") {
    var addList = act.list === "school" ? "school" : "fun"
    var other = addList === "fun" ? "school" : "fun"
    p[other] = (p[other] || []).filter(function (row) { return row.id !== act.id })
    var exists = false
    var rows = p[addList] || []
    for (var ai = 0; ai < rows.length; ai++) {
      if (rows[ai].id === act.id) exists = true
    }
    if (!exists && act.id) {
      rows = rows.concat([{ id: act.id, name: act.name || act.id, kind: act.thingKind || "app" }])
    }
    p[addList] = rows
  } else if (kind === "removeThing") {
    var rmList = act.list === "school" ? "school" : "fun"
    p[rmList] = (p[rmList] || []).filter(function (row) { return row.id !== act.id })
  }
  look.policy = p
  return look
}

function lookToWire(parsedLook) {
  var look = parsedLook || parseLook(null)
  var raw = clone(look.raw || emptyLookRaw())
  var policy = look.policy || defaultPolicy()
  raw.things = []
  raw.apps = {}
  var dur = nightLen(policy.bed, policy.up)
  raw.bedtime = { lights_out: policy.bed, duration: dur }
  var keys = ["mon", "tue", "wed", "thu", "fri", "sat", "sun"]
  raw.fun_hours = {}
  for (var d = 0; d < 7; d++) raw.fun_hours[keys[d]] = Number(policy.funDay[d]) || 0
  if (!raw.pile_hours) raw.pile_hours = {}
  raw.pile_hours.fun = Number(policy.funDay[weekdayMondayFirst(new Date())]) || raw.pile_hours.fun || 3600
  if (!raw.piles || !raw.piles.length) raw.piles = [{ id: "fun", name: "Fun" }]
  if (!raw.modes || !raw.modes.length) raw.modes = [{ id: "freetime", name: "Freetime", kind: "freetime", hours: {} }]
  if (!raw.schedule) raw.schedule = { mon: [], tue: [], wed: [], thu: [], fri: [], sat: [], sun: [] }
  delete raw.catalog
  delete raw.matchers
  return raw
}

function fixtureKid(id) {
  if (id === "bea") {
    return {
      name: "Bea",
      reachable: true,
      status: {
        parentLocked: false,
        bedtimeActive: false,
        bedtimeStart: 20 * 60,
        bedtimeEnd: 7 * 60,
        focusedApp: "khan",
        funLeft: 40 * 60,
        spentFun: 20 * 60,
        spentSchool: 130 * 60,
        today: [
          { kind: "on", start: Math.round((8 + 5 / 60) * 60), dur: 55, label: "Chrome" },
          { kind: "on", start: 12 * 60, dur: 20, label: "YouTube" },
          { kind: "on", start: Math.round((14 + 10 / 60) * 60), dur: 75, label: "Chrome" }
        ]
      },
      look: {
        policy: {
          bed: 20 * 60,
          up: 7 * 60,
          funDay: [3600, 3600, 3600, 3600, 3600, 5400, 5400],
          catalog: [],
          fun: [],
          school: []
        },
        raw: emptyLookRaw()
      },
      asks: [{ id: "bea-ask", seconds: 600 }]
    }
  }
  return {
    name: "Ada",
    reachable: true,
    status: {
      parentLocked: false,
      bedtimeActive: false,
      bedtimeStart: 21 * 60,
      bedtimeEnd: 7 * 60,
      focusedApp: "minecraft",
      funLeft: 16 * 60,
      spentFun: 44 * 60,
      spentSchool: 30 * 60,
      today: [
        { kind: "on", start: Math.round((7 + 40 / 60) * 60), dur: 30, label: "Chrome" },
        { kind: "on", start: Math.round((12 + 10 / 60) * 60), dur: 15, label: "Chrome" },
        { kind: "on", start: Math.round((15 + 58 / 60) * 60), dur: 44, label: "Minecraft" },
        { kind: "on", start: Math.round((16 + 5 / 60) * 60), dur: 12, label: "Chrome" }
      ]
    },
    look: {
      policy: {
        bed: 21 * 60,
        up: 7 * 60,
        funDay: WEEK_DEFAULT.slice(),
        catalog: [],
        fun: [],
        school: []
      },
      raw: emptyLookRaw()
    },
    asks: [{ id: "ada-ask", seconds: 600 }]
  }
}

function fixtureSnapshots() {
  return [fixtureKid("ada"), fixtureKid("bea")]
}

function fixtureTape(kidId, chrome, extra) {
  var snaps = fixtureSnapshots()
  if (extra && extra.locked) {
    snaps[0].status.parentLocked = true
    snaps[0].status.focusedApp = ""
  }
  if (extra && extra.claimed) {
    snaps.push({ name: "Max", claimed: true, reachable: true, status: {} })
  }
  var idx = 0
  if (kidId === "bea") idx = 1
  if (kidId === "max") idx = snaps.length - 1
  var ch = chrome || chromeHome()
  var now = extra && extra.now != null ? extra.now : FIXTURE_NOW
  var household = { pinSet: !(extra && extra.pinSet === false), hour12: extra && extra.hour12 }
  if (extra && extra.kidPin) snaps[idx].status.parentPinSet = true
  return projectTape(snaps, idx, ch, now, household)
}

function asksForBell(snapshots) {
  return householdAsks(snapshots)
}

function askTotal(snapshots) {
  return householdAsks(snapshots).length
}
