.pragma library

function kidtimerBin(settings, home, bundled) {
  var s = settings || {}
  if (s.kidBin) return String(s.kidBin)
  if (s.parentBin) return String(s.parentBin)
  var sys = "/usr/local/bin/kidtimer"
  if (binPresent(sys)) return sys
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

var ASK_MIN = 5
var ASK_MAX = 120
var ASK_STEP = 5
var ASK_DEFAULT_MIN = 30
var OVERLAY_ASK_MIN = 10
var OVERLAY_ASK_STEP = 10

function remainingFor(groups, id) {
  if (!groups || !id) return 0
  var v = groups[id]
  if (v === undefined || v === null) return 0
  return Number(v) || 0
}

function formatMinutes(seconds) {
  var n = Math.floor(Number(seconds) / 60)
  if (isNaN(n) || n < 0) n = 0
  var h = Math.floor(n / 60)
  var m = n % 60
  if (h > 0 && m > 0) return h + "h " + m + "min"
  if (h > 0) return h + "h"
  return m + "min"
}

function pileDisplayName(id, status) {
  var list = pileList(status)
  for (var i = 0; i < list.length; i++) {
    if (list[i].id === id) return list[i].name || id
  }
  return id || ""
}

function copyPiles(src) {
  var out = []
  if (!src) return out
  var n = Number(src.length)
  if (!(n > 0)) return out
  for (var i = 0; i < n; i++) {
    var p = src[i]
    if (!p) continue
    var id = p.id
    if (id === undefined || id === null) id = p["id"]
    var name = p.name
    if (name === undefined || name === null) name = p["name"]
    out.push({ id: String(id || ""), name: String(name || id || "") })
  }
  return out
}

function copyGroups(src) {
  var out = {}
  if (!src) return out
  for (var k in src) out[k] = Number(src[k]) || 0
  return out
}

function parseStatus(raw) {
  var src = raw || {}
  var out = {}
  out.piles = copyPiles(src.piles)
  out.groups = copyGroups(src.groups)
  out.spent = copyGroups(src.spent)
  out.focused_group = src.focused_group
  out.focused_app = src.focused_app
  out.bedtime_active = !!src.bedtime_active
  out.parent_locked = !!src.parent_locked
  out.mode = src.mode
  out.pending_ask_count = Number(src.pending_ask_count) || 0
  out.bedtime_in = src.bedtime_in
  out.bedtime_start = src.bedtime_start
  out.bedtime_end = src.bedtime_end
  out.path_remaining = copyGroups(src.path_remaining)
  out.parent_pin_set = !!src.parent_pin_set
  out.overlay = !!src.overlay
  out.bedtime_hold = !!src.bedtime_hold
  out.hour12 = src.hour12 !== false
  return out
}

function pileList(status) {
  if (!status) return []
  return copyPiles(status.piles)
}

function modeId(status) {
  var m = status && status.mode
  if (m === undefined || m === null || m === "") return ""
  return String(m)
}

function isFreetime(status) {
  return modeId(status) === "freetime"
}

function panelKind(status) {
  if (status && status.bedtime_active) return "bedtime"
  if (status && status.parent_locked) return "locked"
  return "home"
}

function barLabel(status) {
  if (!status) return "kidtimer"
  if (status.bedtime_active) return "bedtime"
  if (status.parent_locked) return "locked"
  return formatMinutes(remainingFor(status.groups, "fun")) + " left"
}

function barUrgent(status) {
  if (!status) return false
  if (status.bedtime_active || status.parent_locked) return true
  var bed = bedtimeIn(status)
  if (bed !== null && bed <= 600) return true
  return remainingFor(status.groups, "fun") <= 600
}

function panelCaption(status) {
  var kind = panelKind(status)
  if (kind === "bedtime") return "bedtime"
  if (kind === "locked") return "locked"
  return ""
}

function clockLabel(min, hour12) {
  min = ((Math.round(Number(min)) % 1440) + 1440) % 1440
  var h = Math.floor(min / 60)
  var m = min % 60
  if (hour12 === false) {
    var hs = String(h)
    var ms = String(m)
    if (hs.length < 2) hs = "0" + hs
    if (ms.length < 2) ms = "0" + ms
    return hs + ":" + ms
  }
  var ap = h >= 12 ? "PM" : "AM"
  var hr = ((h + 11) % 12) + 1
  var mm = String(m)
  if (mm.length < 2) mm = "0" + mm
  return hr + ":" + mm + " " + ap
}

function clockFromHHMM(hhmm, hour12) {
  var p = String(hhmm || "").split(":")
  var h = Number(p[0]) || 0
  var m = Number(p[1]) || 0
  return clockLabel(h * 60 + m, hour12)
}

function bedtimeEnd(status) {
  if (!status || status.bedtime_end === undefined || status.bedtime_end === null) return ""
  return clockFromHHMM(status.bedtime_end, status.hour12)
}

function bedtimeStart(status) {
  if (!status || status.bedtime_start === undefined || status.bedtime_start === null) return ""
  return clockFromHHMM(status.bedtime_start, status.hour12)
}

function bedtimeBanner(status) {
  if (!status || status.bedtime_active) return ""
  if (status.bedtime_in === undefined || status.bedtime_in === null) return ""
  var n = Number(status.bedtime_in)
  if (!(n > 0)) return ""
  var at = bedtimeStart(status)
  if (!at) return ""
  return "bedtime starts at " + at
}

function clockEmpty(status) {
  return remainingFor(status && status.groups, "fun") <= 0
}

function askBlocked(status) {
  return false
}

function overlayAskWaiting(status) {
  return (Number(status && status.pending_ask_count) || 0) > 0
}

function askPayload(group, seconds, reason) {
  var g = group || "fun"
  var s = Number(seconds)
  if (!(s > 0)) s = ASK_DEFAULT_MIN * 60
  return { group: g, seconds: s, reason: reason || "more time" }
}

function clampAskMinutes(n) {
  var m = Math.round(Number(n) / ASK_STEP) * ASK_STEP
  if (isNaN(m)) m = ASK_DEFAULT_MIN
  if (m < ASK_MIN) m = ASK_MIN
  if (m > ASK_MAX) m = ASK_MAX
  return m
}

function nudgeAskMinutes(current, delta) {
  return clampAskMinutes((Number(current) || ASK_DEFAULT_MIN) + Number(delta))
}

function clampOverlayAskMinutes(n) {
  var m = Math.round(Number(n) / OVERLAY_ASK_STEP) * OVERLAY_ASK_STEP
  if (isNaN(m)) m = ASK_DEFAULT_MIN
  if (m < OVERLAY_ASK_MIN) m = OVERLAY_ASK_MIN
  if (m > ASK_MAX) m = ASK_MAX
  return m
}

function nudgeOverlayAskMinutes(current, delta) {
  return clampOverlayAskMinutes((Number(current) || ASK_DEFAULT_MIN) + Number(delta))
}

function askGroups(status) {
  var list = pileList(status)
  var ids = []
  for (var i = 0; i < list.length; i++) ids.push(list[i].id)
  return ids
}

function fillPercent(remaining) {
  var n = Number(remaining)
  if (!(n > 0)) return 0
  return Math.min(1, n / 3600)
}

function spentFor(status) {
  return remainingFor(status && status.spent, "fun")
}

function allotmentFor(status) {
  var left = remainingFor(status && status.groups, "fun")
  var used = spentFor(status)
  var n = left + used
  return n > 0 ? n : 3600
}

function clockFill(status) {
  var left = remainingFor(status && status.groups, "fun")
  if (!(left > 0)) return 0
  return Math.min(1, left / allotmentFor(status))
}

function clockFace(status, waiting) {
  var left = remainingFor(status && status.groups, "fun")
  return {
    leftLabel: formatMinutes(left) + " LEFT",
    fill: clockFill(status),
    empty: left <= 0,
    waiting: !!waiting
  }
}

function groupRows(status, waitingGroup) {
  var groups = status && status.groups
  var focused = status && status.focused_group
  var list = pileList(status)
  var rows = []
  for (var i = 0; i < list.length; i++) {
    var id = list[i].id
    var remaining = remainingFor(groups, id)
    var fill = fillPercent(remaining)
    rows.push({
      id: id,
      name: list[i].name || pileDisplayName(id, status),
      remaining: remaining,
      fill: fill,
      low: fill <= 0.15,
      focused: focused === id,
      waiting: waitingGroup === id
    })
  }
  return rows
}

function warnSeconds() {
  return [900, 300, 60]
}

function pathRemainingFor(status, group) {
  if (!status || !status.path_remaining || !group) return null
  if (!Object.prototype.hasOwnProperty.call(status.path_remaining, group)) return null
  var n = Number(status.path_remaining[group])
  if (isNaN(n)) return 0
  return n
}

function bedtimeIn(status) {
  if (!status || status.bedtime_in === undefined || status.bedtime_in === null) return null
  if (status.bedtime_active) return 0
  var n = Number(status.bedtime_in)
  if (isNaN(n)) return null
  return n
}

function crossedWarning(prev, next) {
  var p = Number(prev)
  var n = Number(next)
  if (!(p > 0) || isNaN(n)) return 0
  var hit = 0
  var thresholds = warnSeconds()
  for (var i = 0; i < thresholds.length; i++) {
    var t = thresholds[i]
    if (p > t && n <= t && n > 0) hit = t
  }
  return hit
}

function warningCopy(kind, seconds, status) {
  if (kind === "bedtime") {
    var at = bedtimeStart(status)
    if (at) return "bedtime starts at " + at
    return ""
  }
  var mins = Math.round(Number(seconds) / 60)
  return mins + " min left"
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

function overlayPinAdvance(digits) {
  if (!validPin(digits)) {
    return { ok: false, step: "pin", chosenMinutes: ASK_DEFAULT_MIN }
  }
  return { ok: true, step: "minutes", chosenMinutes: ASK_DEFAULT_MIN }
}

function parentPinLabel() {
  return "Parent Pin"
}

function overlayVisible(status) {
  return !!(status && status.overlay)
}

function overlayFace(status) {
  if (!overlayVisible(status)) return ""
  if (status.parent_locked) return "locked"
  if (status.bedtime_active) return "bedtime"
  if (remainingFor(status.groups, "fun") <= 0) return "empty"
  return "locked"
}

function pinApprovePayload(pin, askId) {
  return { pin: String(pin || ""), ask_id: String(askId || "") }
}

function pinGrantPayload(pin, seconds) {
  var s = Number(seconds)
  if (!(s > 0)) s = ASK_DEFAULT_MIN * 60
  return { pin: String(pin || ""), seconds: s }
}

function overlayStepperLabel(minutes) {
  return String(clampAskMinutes(minutes))
}

function overlayAskStepperLabel(minutes) {
  return String(clampOverlayAskMinutes(minutes))
}

function bankSetting(bank, settings, key, fallback) {
  if (bank && bank[key]) return String(bank[key])
  if (settings && settings[key]) return String(settings[key])
  if (fallback === undefined || fallback === null) return ""
  return fallback
}

function parseKidBank(raw) {
  var src = raw || {}
  var out = { url: "http://127.0.0.1:8742", readToken: "", askToken: "" }
  if (src.url) out.url = String(src.url).replace(/\/$/, "")
  if (src.readToken) out.readToken = String(src.readToken)
  if (src.askToken) out.askToken = String(src.askToken)
  return out
}

function kidSettingsFromShell(doc) {
  var out = { url: "http://127.0.0.1:8742", readToken: "", askToken: "" }
  if (!doc || !doc.bar || !doc.bar.layout) return out
  var layout = doc.bar.layout
  var names = ["left", "center", "right"]
  for (var n = 0; n < names.length; n++) {
    var list = layout[names[n]] || []
    for (var i = 0; i < list.length; i++) {
      var row = list[i] || {}
      if (row.id !== "io.github.adam-lagerhausen.kidtimer" && row.id !== "kidtimer" && row.id !== "kidtimer.kid") continue
      if (row.url) out.url = String(row.url).replace(/\/$/, "")
      if (row.readToken) out.readToken = String(row.readToken)
      if (row.askToken) out.askToken = String(row.askToken)
      return out
    }
  }
  return out
}

function emptyWarnState() {
  return { seeded: false, remaining: {}, bedtimeIn: null }
}

function takeWarnings(state, status) {
  var prev = state || emptyWarnState()
  var remaining = {}
  var keys = Object.keys(prev.remaining || {})
  for (var i = 0; i < keys.length; i++) {
    remaining[keys[i]] = prev.remaining[keys[i]]
  }
  var nextBed = bedtimeIn(status)
  if (!prev.seeded) {
    var g0 = status && status.focused_group
    var path0 = pathRemainingFor(status, g0)
    if (g0 && path0 !== null) remaining[g0] = path0
    return { state: { seeded: true, remaining: remaining, bedtimeIn: nextBed }, notices: [] }
  }
  var notices = []
  var group = status && status.focused_group
  var path = pathRemainingFor(status, group)
  if (group && path !== null) {
    if (!Object.prototype.hasOwnProperty.call(remaining, group)) {
      remaining[group] = path
    } else {
      var hit = crossedWarning(remaining[group], path)
      remaining[group] = path
      if (hit) notices.push(warningCopy(group, hit))
    }
  }
  if (nextBed !== null && prev.bedtimeIn !== null) {
    var bedHit = crossedWarning(prev.bedtimeIn, nextBed)
    if (bedHit) {
      var bedCopy = warningCopy("bedtime", bedHit, status)
      if (bedCopy) notices.push(bedCopy)
    }
  }
  return { state: { seeded: true, remaining: remaining, bedtimeIn: nextBed }, notices: notices }
}
