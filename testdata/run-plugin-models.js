#!/usr/bin/env node
const fs = require("fs")
const path = require("path")
const vm = require("vm")

function load(rel) {
  const file = path.join(__dirname, "..", rel)
  const src = fs.readFileSync(file, "utf8").replace(/^\.pragma library\s*/m, "")
  const ctx = {}
  vm.runInNewContext(src, ctx)
  return ctx
}

function assertEqual(got, want, msg) {
  const a = JSON.stringify(got)
  const b = JSON.stringify(want)
  if (a !== b) {
    console.error(msg + ": got " + a + " want " + b)
    process.exit(1)
  }
}

const kid = load("KidModel.js")
const parent = load("ParentModel.js")

const classicPiles = [
  { id: "minecraft", name: "Games" },
  { id: "youtube", name: "YouTube" },
  { id: "fun", name: "Fun" }
]

assertEqual(kid.barLabel({ bedtime_active: true }), "bedtime", "bedtime")
assertEqual(kid.barLabel({ bedtime_active: false, focused_group: null, groups: { fun: 0 } }), "0min left", "idle null")
assertEqual(kid.barLabel({ bedtime_active: false, groups: { fun: 2820 } }), "47min left", "remaining only")
assertEqual(kid.barLabel({
  bedtime_active: false,
  focused_app: "minecraft",
  focused_group: "fun",
  groups: { fun: 960 }
}), "16min left", "countdown")
assertEqual(kid.barLabel({
  bedtime_active: false,
  focused_app: "minecraft",
  groups: { fun: 5400 }
}), "1h 30min left", "hour plus minutes")
assertEqual(kid.askPayload("", 0).group, "fun", "ask default group")
assertEqual(kid.askPayload("", 0).seconds, 1800, "ask default seconds")
assertEqual(kid.fillPercent(0), 0, "fill 0")
assertEqual(kid.fillPercent(1800), 0.5, "fill 1800")
assertEqual(kid.fillPercent(3600), 1, "fill 3600")
assertEqual(kid.fillPercent(7200), 1, "fill 7200")

const rowStatus = {
  bedtime_active: false,
  piles: classicPiles,
  groups: { school: 7200, youtube: 3600, fun: 0, minecraft: 1800 }
}
const rows = kid.groupRows(rowStatus)
assertEqual(rows.map(r => r.id), ["minecraft", "youtube", "fun"], "groupRows order")
assertEqual(rows.map(r => r.name), ["Games", "YouTube", "Fun"], "groupRows names")
assertEqual(rows.map(r => r.remaining), [1800, 3600, 0], "groupRows remaining")
assertEqual(rows.map(r => r.fill), [0.5, 1, 0], "groupRows fill")
assertEqual(rows.map(r => r.low), [false, false, true], "groupRows low")
assertEqual(rows.some(r => r.id === "school"), false, "school omitted")
assertEqual(kid.groupRows({
  bedtime_active: true,
  piles: classicPiles,
  groups: rowStatus.groups
}).map(r => r.id), rows.map(r => r.id), "bedtime does not change rows")
assertEqual(kid.groupRows({ groups: rowStatus.groups }).map(r => r.id), [], "groups without piles")
assertEqual(kid.groupRows({ piles: [], groups: rowStatus.groups }).map(r => r.id), [], "empty look piles")
assertEqual(kid.groupRows({
  piles: [{ id: "games", name: "Games" }],
  groups: { games: 1200, minecraft: 1800 }
}).map(r => r.id), ["games"], "look piles win")
assertEqual(kid.askGroups({ piles: [] }), [], "askGroups empty piles")
assertEqual(kid.parseStatus({
  piles: [{ id: "new", name: "Minecraft" }],
  groups: { new: 0, minecraft: 3600 }
}).piles.map(p => p.id), ["new"], "parseStatus piles")
assertEqual(kid.groupRows(kid.parseStatus({
  piles: [{ id: "new", name: "Minecraft" }, { id: "new-2", name: "Fun" }],
  groups: { new: 0, "new-2": 0, minecraft: 3600 }
})).map(r => r.name), ["Minecraft", "Fun"], "parsed status drives rows")
assertEqual(kid.groupRows({}).map(r => r.id), [], "empty status is not classic piles")

assertEqual(parent.defaultGroup({ fun: 0, minecraft: 60 }), "fun", "default fun")
assertEqual(parent.grantPayload("", 600, { fun: 0 }).group, "fun", "grant default fun")
assertEqual(parent.grantPayload("minecraft", 600).seconds, 600, "+10")
assertEqual(parent.grantPayload("fun", 600).reason, "+10", "+10 reason")
assertEqual(parent.grantPayload("fun", -600).seconds, -600, "-10 seconds")
assertEqual(parent.grantPayload("fun", -600).reason, "-10", "-10 reason")
assertEqual(parent.newAskIds(["a"], { asks: [{ id: "a" }, { id: "b" }] }), ["b"], "new id")
assertEqual(parent.newAskIds(["a"], { asks: [{ id: "a" }] }), [], "no new")
assertEqual(parent.parentBin({}, "/home/parent"), "/home/parent/.local/bin/kidtimer", "parent bin default")
assertEqual(parent.parentBin({ parentBin: "/opt/kidtimer" }, "/home/parent"), "/opt/kidtimer", "parent bin override")
assertEqual(parent.kidtimerBin({ kidBin: "/opt/k" }, "/home/parent", "/bundled"), "/opt/k", "kid bin override")
assertEqual(parent.kidtimerBin({}, "", "/plugin/bin/kidtimer"), "/plugin/bin/kidtimer", "bundled bin")
assertEqual(kid.kidtimerBin({ kidBin: "/opt/k" }, "/home/parent", "/bundled"), "/opt/k", "kid model bin")
assertEqual(kid.kidtimerBin({}, "/home/kid", "/bundled"), "/usr/local/bin/kidtimer", "kid system bin")
assertEqual(parent.kidsFromSettings({})[0].url, "http://127.0.0.1:8742", "localhost")
assertEqual(parent.notifyHeadline("Ada"), "Ada", "notify who")
assertEqual(parent.notifyHeadline("<Ada & Bea>"), "Ada  Bea", "notify strips host markup")
assertEqual(parent.householdBarLabel([{ name: "Ada<script>" }]), "Adascript", "bar strips host markup")
assertEqual(parent.lockLabel("Ada", false), "Lock Ada", "lock Ada")
assertEqual(parent.lockLabel("Ada", true), "Unlock Ada", "unlock Ada")
assertEqual(parent.lockPayload(true), { locked: true }, "lock on")
assertEqual(parent.lockPayload(false), { locked: false }, "lock off")
assertEqual(parent.bedtimePayload("20:00", "07:00"), {
  bedtime_start: "20:00",
  bedtime_end: "07:00"
}, "bedtime payload")
assertEqual(typeof parent.THEME, "undefined", "no THEME")
assertEqual(typeof parent.modePayload, "undefined", "no modePayload")
assertEqual(typeof parent.freetimeUntil, "undefined", "no freetimeUntil")
assertEqual(typeof parent.remainingRows, "undefined", "no remainingRows")

const twoKids = parent.kidsFromSettings({
  parentToken: "pt",
  kids: [
    { name: "Ada", url: "http://a:8742" },
    { name: "Bea", url: "http://b:8742", token: "bea" }
  ]
})
assertEqual(twoKids.map(k => k.name), ["Ada", "Bea"], "two kids names")
assertEqual(twoKids[0].token, "pt", "kid token fallback")
assertEqual(twoKids[1].token, "bea", "kid token row")
const discovered = parent.parseHousehold(JSON.stringify({
  kids: [
    { id: "m1", name: "ada-box", url: "http://192.168.1.20:8742", token: "pair" },
    { id: "m1", name: "dup", url: "http://192.168.1.21:8742", token: "x" }
  ]
}))
assertEqual(discovered[0].name, "ada-box", "parse household")
assertEqual(parent.mergeKids({}, []).length, 0, "empty household no pin")
assertEqual(parent.mergeKids({
  kids: [{ name: "kid-a", url: "http://127.0.0.1:8742" }]
}, []).length, 0, "settings kids stay off tape")
assertEqual(parent.mergeKids({
  lab: true,
  kids: [{ name: "lab", url: "http://127.0.0.1:8742" }]
}, []).map(k => k.name), ["lab"], "lab pin")
const merged = parent.mergeKids({
  lab: true,
  parentToken: "pt",
  kids: [
    { name: "Ada", url: "http://a:8742" },
    { name: "Bea", url: "http://b:8742", token: "bea" }
  ]
}, [{ id: "m1", name: "ada-box", url: "http://192.168.1.20:8742", token: "pair" }])
assertEqual(merged.map(k => k.name), ["Ada", "Bea", "ada-box"], "lab pins plus pair")
assertEqual(merged[2].token, "pair", "pair token")
const onlyPair = parent.mergeKids({}, [{ id: "m1", name: "ada-box", url: "http://192.168.1.20:8742", token: "pair" }])
assertEqual(onlyPair.map(k => k.name), ["ada-box"], "no localhost fallback when paired")
const again = parent.mergeKids({ lab: true }, [
  { id: "m1", name: "ada-box", url: "http://192.168.1.20:8742", token: "pair" },
  { id: "m1", name: "ada-box", url: "http://192.168.1.20:8742", token: "pair" }
])
assertEqual(again.length, 1, "machine-id reuse")
assertEqual(parent.householdBarLabel([]), "no computers", "empty household bar")
assertEqual(parent.householdBarLabel([
  { name: "Ada", reachable: true, status: { mode: "evening" }, asks: [] },
  { name: "Bea", reachable: true, status: {}, asks: [{ id: "a1" }] }
]), "Ada · Bea", "household bar")
const pinFirst = parent.projectTape([], 0, parent.chromeHome(), null, { pinSet: false })
assertEqual(pinFirst.needsPin, true, "empty household needs pin")
assertEqual(pinFirst.waiting, false, "pin screen is not waiting")
assertEqual(pinFirst.showLock, false, "pin screen hides lock")
const waitingTape = parent.projectTape([], 0, parent.chromeHome(), null, { pinSet: true })
assertEqual(waitingTape.waiting, true, "empty tape waiting")
assertEqual(waitingTape.needsPin, false, "pin already set")
assertEqual(waitingTape.kid.name, "", "empty tape has no kid name")
assertEqual(waitingTape.howTo.title, "No computers found", "empty how-to title")
assertEqual(waitingTape.howTo.lines.map(l => l.text), [
  "Run this on the kid computer. It will show up here.",
  "omarchy plugin add https://github.com/adam-lagerhausen/omarchy-kidtimer.git --enable"
], "empty how-to lines")
assertEqual(waitingTape.howTo.lines.some(l => l.cmd), true, "empty how-to shows the kid install command")
assertEqual(parent.projectTape(parent.fixtureSnapshots(), 0, parent.chromeHome(), null, { pinSet: false }).needsPin, true, "kids without pin still need pin")
assertEqual(parent.projectTape(parent.fixtureSnapshots(), 0, parent.chromeHome(), null, { pinSet: true }).needsPin, false, "kids with pin skip pin screen")
assertEqual(parent.projectTape(parent.fixtureSnapshots(), 0, parent.chromeHome(), null, { pinSet: true }).waiting, false, "kids with pin show home")

const claimedRows = parent.parseHousehold({
  kids: [],
  seen: [{ id: "kid-1", name: "testMax", url: "http://100.64.1.2:8742", claimed: true }]
})
assertEqual(claimedRows.length, 1, "seen rows in household")
assertEqual(claimedRows[0].claimed, true, "seen claimed")
assertEqual(claimedRows[0].token, "", "seen has no token")
const mixedHH = parent.parseHousehold({
  kids: [{ id: "m1", name: "ada-box" }],
  seen: [{ id: "kid-1", name: "testMax", url: "http://100.64.1.2:8742", claimed: true }]
})
assertEqual(mixedHH.map(k => k.name), ["ada-box", "testMax"], "ours plus claimed")
const claimedTape = parent.projectTape([{
  name: "testMax", claimed: true, reachable: true, status: {}
}], 0, parent.chromeHome(), null, { pinSet: true })
assertEqual(claimedTape.waiting, false, "claimed-only is not waiting")
assertEqual(claimedTape.howTo.title || "", "", "claimed-only has no empty how-to")
assertEqual(claimedTape.ours, false, "claimed-only is not ours")
assertEqual(claimedTape.showLock, false, "claimed-only hides lock")
assertEqual(claimedTape.kid.face.caption, "Already claimed", "claimed caption")
assertEqual(claimedTape.kids.length, 1, "claimed stays in picker")
const pickClaimed = parent.reduceChrome(claimedTape.chrome, { kind: "pick" }, claimedTape)
assertEqual(pickClaimed.chrome.picker, true, "claimed picker opens")
const selClaimed = parent.reduceChrome(pickClaimed.chrome, { kind: "select", kidIndex: 0 }, claimedTape)
assertEqual(selClaimed.chrome.picker, false, "claimed select closes picker")
assertEqual(selClaimed.selectIndex, 0, "claimed select aims")
assertEqual(selClaimed.chrome.adopt.index, 0, "claimed select asks adopt")
const adoptNo = parent.reduceChrome(selClaimed.chrome, { kind: "adoptNo" }, claimedTape)
assertEqual(adoptNo.chrome.adopt, null, "no closes prompt")
assertEqual(claimedTape.kids.length, 1, "no keeps the row")
const adoptYes = parent.reduceChrome(selClaimed.chrome, { kind: "adoptYes" }, claimedTape)
assertEqual(adoptYes.adopt.index, 0, "yes takeovers")
assertEqual(adoptYes.chrome.adopt, null, "yes closes prompt")
const oursTape = parent.projectTape(parent.fixtureSnapshots(), 0, parent.chromeHome(), null, { pinSet: true })
const oursSel = parent.reduceChrome(oursTape.chrome, { kind: "select", kidIndex: 1 }, oursTape)
assertEqual(oursSel.chrome.adopt, null, "ours click does not adopt")
assertEqual(oursSel.selectIndex, 1, "ours click selects")

function assertFace(snap, kind, cap, msg) {
  const f = parent.hostFace(snap)
  assertEqual(f.kind, kind, msg + " kind")
  assertEqual(parent.computerCaption(f), cap, msg + " cap")
  assertEqual(f.led, kind === "active", msg + " led")
}
assertFace({ reachable: false, status: { bedtime_active: true, parent_locked: true, focused_app: "minecraft" } }, "offline", "Offline", "offline")
assertFace({ error: true, reachable: false, status: { bedtime_active: true, parent_locked: true, focused_app: "minecraft" } }, "error", "Error", "error")
assertFace({ reachable: true, status: { bedtime_active: true, parent_locked: true, focused_app: "minecraft" } }, "locked", "Locked", "locked beats bedtime")
assertFace({ reachable: true, status: { bedtime_active: true, focused_app: "minecraft" } }, "bedtime", "Bedtime", "bedtime")
assertFace({ reachable: true, status: { parent_locked: true, focused_app: "minecraft" } }, "locked", "Locked", "locked")
assertFace({ reachable: true, status: { focused_app: "minecraft" } }, "active", "Active · minecraft", "active app")
assertEqual(parent.hostFace({ reachable: true, status: { focused_app: "minecraft" } }).doing, "minecraft", "doing app")
assertEqual(parent.hostFace({ reachable: true, status: { mode: "freetime", focused_app: "minecraft" } }).doing, "minecraft", "mode is not a caption")
assertEqual(parent.hostFace({ reachable: true, status: {} }).doing, "", "doing active no app")
assertEqual(parent.hostFace({ reachable: false, status: {} }).doing, "", "doing offline")
assertFace({ reachable: true, status: {} }, "active", "Active", "active")
assertEqual(parent.hostFace({ error: true, reachable: false, status: {} }).coral, true, "error coral")
const errKid = parent.projectKid({
  name: "Ada",
  reachable: true,
  error: true,
  status: { bedtime_active: true, parent_locked: true, focused_app: "minecraft" }
}, 0, new Date())
assertEqual(errKid.face.kind, "error", "projectKid error kind")
assertEqual(errKid.face.caption, "Error", "projectKid error cap")
assertEqual(errKid.pickerLine, "○ ADA  Error", "picker is dot, name, status")

const home = parent.fixtureTape("ada", parent.chromeHome())
assertEqual(home.theme, undefined, "tape has no theme")
assertEqual(home.kid.nameUp, "ADA", "fixture ada")
assertEqual(home.kid.face.caption, "Active · minecraft", "ada active minecraft")
assertEqual(home.kid.pickerLine, "● ADA  Active · minecraft", "home picker is dot, name, status")
assertEqual(home.kid.fun.usedLabel, "44m", "ada used")
assertEqual(home.kid.fun.leftLabel, "16m LEFT", "ada left")
assertEqual(home.kid.fun.empty, false, "ada not empty")
assertEqual(home.kid.fun.barLow, false, "ada bar not low")
assertEqual(home.bellCount, 2, "household asks")
assertEqual(home.showLock, true, "home lock")
assertEqual(home.showStamp, false, "home no stamp")
assertEqual(home.track.log[0].clock, "7:40 AM", "log clock")
assertEqual(home.track.log[0].name, "CHROME", "log chrome")
assertEqual(home.track.log[2].name, "MINECRAFT + CHROME", "log sitting names")
assertEqual(home.track.hours[0], "12a", "12h track start")
assertEqual(home.hour12, true, "default 12 hour")
const home24 = parent.fixtureTape("ada", parent.chromeHome(), { hour12: false })
assertEqual(home24.track.log[0].clock, "07:40", "24h log clock")
assertEqual(home24.track.hours[0], "0", "24h track start")
assertEqual(home.track.needle != null, true, "home needle")
assertEqual(home.track.blocks.length, 3, "home activity blocks")
assertEqual(Math.round(home.track.blocks[1].widthPct), Math.round(15 / 1440 * 100), "fifteen minutes is a sliver")

const spentOnly = parent.projectTape([{
  name: "Ada",
  reachable: true,
  status: parent.parseStatus({
    spent: { fun: 410 },
    groups: { fun: 6790 },
    bedtime_start: "22:00",
    bedtime_end: "06:00"
  })
}], 0, parent.chromeHome(), 7.5, { pinSet: true })
assertEqual(spentOnly.kid.fun.usedLabel, "6m", "spent used")
assertEqual(spentOnly.track.blocks.length, 1, "spent becomes a block")
assertEqual(spentOnly.track.log[0].dur, "6m", "spent log dur")
assertEqual(spentOnly.track.log[0].name, "ON", "spent log name")
assertEqual(parent.askCardText("Ada", 1800), "Ada asked for 30 more minutes", "ask minutes copy")
assertEqual(parent.askCardText("Ada", 1800, true), "Ada asked to unlock", "locked ask copy")
assertEqual(parent.notifySummary({ seconds: 1800 }, null, { parentLocked: true }), "unlock", "locked notify")
assertEqual(parent.notifySummary({ seconds: 1800 }, null, { parent_locked: true }), "unlock", "locked notify wire")
assertEqual(parent.notifySummary({ seconds: 1800 }, null, {}), "+30m", "unlocked notify")
assertEqual(home.asks[0].text, "Ada asked for 10 more minutes", "ask copy")

const bea = parent.fixtureTape("bea", parent.chromeHome())
assertEqual(bea.kid.nameUp, "BEA", "bea")
assertEqual(bea.kid.face.caption, "Active · khan", "bea active khan")
assertEqual(bea.kid.fun.usedLabel, "20m", "bea used")
assertEqual(bea.kid.fun.leftLabel, "40m LEFT", "bea left")
assertEqual(bea.kid.policy.bedLabel, "8:00 PM", "bea bed")

const locked = parent.fixtureTape("ada", parent.chromeHome(), { locked: true })
assertEqual(locked.showStamp, true, "locked stamp")
assertEqual(locked.kid.face.coral, true, "locked coral")
assertEqual(locked.kid.face.caption, "Locked", "locked caption")
assertEqual(locked.lockLabel, "Unlock Ada", "unlock ada")
assertEqual(locked.asks[0].text, "Ada asked to unlock", "locked tape ask")

const settings = parent.fixtureTape("ada", parent.chromeSettings())
assertEqual(settings.showLock, false, "settings hide lock")
assertEqual(settings.showStamp, false, "settings hide stamp")
assertEqual(settings.track.needle, null, "settings no needle")
assertEqual(settings.track.blocks.length, 0, "settings no blocks")
assertEqual(settings.kid.policy.bedLabel, "9:00 PM", "ada bed")
assertEqual(settings.kid.policy.upLabel, "7:00 AM", "ada up")
assertEqual(settings.hour12, true, "settings 12 hour")

const clock24 = parent.fixtureTape("ada", parent.chromeSettings(), { hour12: false })
assertEqual(clock24.kid.policy.bedLabel, "21:00", "24h ada bed")
assertEqual(clock24.kid.policy.upLabel, "07:00", "24h ada up")
assertEqual(clock24.hour12, false, "settings 24 hour")
assertEqual(clock24.track.hours.join(" "), "0 6 12 18 24", "24h track")
assertEqual(settings.kid.policy.funDayRows[0].dow, "MON", "mon")
assertEqual(settings.kid.policy.funDayRows[0].label, "1h", "mon 1h")
assertEqual(settings.kid.policy.funDayRows[5].label, "2h", "sat 2h")
assertEqual(settings.kid.policy.catalog.length, 0, "empty catalog")
assertEqual(settings.kid.policy.fun.length, 0, "empty fun")
assertEqual(settings.kid.policy.school.length, 0, "empty school")

const typed = parent.reduceChrome(parent.chromeSettings(), { kind: "query", list: "fun", q: "mine" }, settings)
assertEqual(typed.chrome.query.fun, "mine", "query fun")
assertEqual(typed.chrome.query.school, "", "query school stays")
assertEqual(typed.chrome.hits.fun.length, 0, "query clears fun hits")
const found = parent.reduceChrome(typed.chrome, {
  kind: "hits",
  list: "fun",
  hits: [{ id: "desktop:org.prismlauncher.PrismLauncher", name: "Minecraft", kind: "app" }]
}, settings)
assertEqual(found.chrome.hits.fun[0].name, "Minecraft", "hits fun")
assertEqual(found.chrome.query.fun, "mine", "hits keep query")
const picked = parent.reduceChrome(found.chrome, {
  kind: "addThing",
  list: "fun",
  id: "desktop:org.prismlauncher.PrismLauncher",
  name: "Minecraft",
  thingKind: "app"
}, settings)
assertEqual(picked.chrome.query.fun, "", "add clears query")
assertEqual(picked.chrome.hits.fun.length, 0, "add clears hits")

const gear = parent.reduceChrome(parent.chromeHome(), { kind: "settings" }, home)
assertEqual(gear.chrome.face, "settings", "gear opens settings")
assertEqual(gear.chrome.picker, false, "gear closes picker")
const pick = parent.reduceChrome(parent.chromeHome(), { kind: "pick" }, home)
assertEqual(pick.chrome.picker, true, "pick opens")
const aimed = parent.reduceChrome(pick.chrome, { kind: "select", kidIndex: 1 }, home)
assertEqual(aimed.chrome.picker, false, "select closes picker")
assertEqual(aimed.selectIndex, 1, "select bea")
assertEqual(aimed.chrome.bell, false, "select leaves bell")
assertEqual(parent.grantPayload("fun", 600).group, "fun", "selected kid still grants fun")
assertEqual(parent.lockPayload(true), { locked: true }, "selected kid lock body")
assertEqual(parent.bedtimePayload("20:00", "07:00"), {
  bedtime_start: "20:00",
  bedtime_end: "07:00"
}, "selected kid bedtime body")
assertEqual(twoKids[aimed.selectIndex].url, "http://b:8742", "aimed kid url")
const emptyBell = parent.reduceChrome(parent.chromeHome(), { kind: "bell" }, { bellCount: 0 })
assertEqual(emptyBell.chrome.bell, false, "empty bell stays shut")

const wire = parent.lookToWire(parent.fixtureSnapshots()[0].look)
assertEqual(Object.prototype.hasOwnProperty.call(wire, "catalog"), false, "lookToWire no catalog")
assertEqual(wire.apps.minecraft, undefined, "no catalog slug")
assertEqual((wire.things || []).length, 0, "empty things")

const adaSnap = parent.fixtureSnapshots()[0]
const bedPlus = parent.applyPolicy(adaSnap, { kind: "bed", delta: 15 })
assertEqual(bedPlus.policy.bed, 21 * 60 + 15, "step bed +15")
assertEqual(parent.bedtimePayload(bedPlus.policy.bed, bedPlus.policy.up), {
  bedtime_start: "21:15",
  bedtime_end: "07:00"
}, "stepped bedtime patch")
const bedFromStatus = JSON.parse(JSON.stringify(adaSnap))
bedFromStatus.look.policy.bed = 21 * 60
bedFromStatus.status.bedtimeStart = 20 * 60
bedFromStatus.status.bedtimeEnd = 7 * 60
const steppedStatus = parent.applyPolicy(bedFromStatus, { kind: "bed", delta: 15 })
assertEqual(steppedStatus.policy.bed, 20 * 60 + 15, "step the clock on screen")
const settingsAfter = parent.projectTape([Object.assign({}, adaSnap, {
  look: steppedStatus,
  status: Object.assign({}, adaSnap.status, { bedtimeStart: steppedStatus.policy.bed, bedtimeEnd: steppedStatus.policy.up })
})], 0, parent.chromeSettings(), null, { pinSet: true })
assertEqual(settingsAfter.kid.policy.bedLabel, "8:15 PM", "settings shows stepped bed")

const added = parent.applyPolicy(parent.fixtureSnapshots()[0], {
  kind: "addThing",
  list: "fun",
  id: "desktop:org.prismlauncher.PrismLauncher",
  name: "Minecraft",
  thingKind: "app"
})
assertEqual(added.policy.fun[0].name, "Minecraft", "add minecraft")
const wiredAdd = parent.lookToWire(added)
assertEqual(wiredAdd.apps["desktop:org.prismlauncher.PrismLauncher"], undefined, "lists do not persist")
assertEqual((wiredAdd.things || []).length, 0, "lookToWire empty things")
assertEqual(wiredAdd.fun_hours.sat, 7200, "sat hours survive add")
assertEqual(Object.prototype.hasOwnProperty.call(wiredAdd, "catalog"), false, "add has no catalog")
assertEqual(wire.bedtime.lights_out, 21 * 60, "bed lights")
assertEqual(wire.fun_hours.sat, 7200, "sat fun hours")


assertEqual(kid.barLabel({
  focused_group: "fun",
  focused_app: "minecraft",
  groups: { fun: 960 }
}), "16min left", "fun caption remaining")
assertEqual(kid.barLabel({
  focused_app: "khan academy",
  groups: { fun: 960 }
}), "16min left", "any app spends")
assertEqual(kid.barUrgent({
  groups: { fun: 0 }
}), true, "empty remaining is urgent")
assertEqual(kid.barLabel({
  parent_locked: true,
  focused_app: "minecraft",
  groups: { fun: 3600 }
}), "locked", "locked")
assertEqual(kid.barLabel({
  bedtime_active: true,
  parent_locked: true,
  focused_app: "minecraft"
}), "bedtime", "bedtime wins lock")
assertEqual(kid.barLabel({ focused_group: null, groups: { fun: 0 } }), "0min left", "empty idle")
assertEqual(kid.panelCaption({ bedtime_active: true }), "bedtime", "caption bedtime")
assertEqual(kid.panelCaption({ parent_locked: true }), "locked", "caption locked")
assertEqual(kid.panelCaption({ bedtime_active: true, parent_locked: true }), "bedtime", "caption bedtime wins")
assertEqual(kid.panelCaption({ mode: "evening" }), "", "status.mode is ignored")
assertEqual(kid.panelCaption({}), "", "caption gap")
assertEqual(kid.panelKind({ bedtime_active: true, parent_locked: true }), "bedtime", "kind bedtime")
assertEqual(kid.panelKind({ parent_locked: true }), "locked", "kind locked")
assertEqual(kid.panelKind({ mode: "evening" }), "home", "leftover mode is home")
assertEqual(kid.panelKind({ mode: "freetime", parent_locked: true }), "locked", "lock beats leftover mode")
assertEqual(kid.clockFace({ groups: { fun: 960 }, spent: { fun: 2640 } }).leftLabel, "16min LEFT", "clock left")
assertEqual(kid.clockFace({ groups: { fun: 960 }, spent: { fun: 2640 } }).fill, 960 / 3600, "clock fill")
assertEqual(kid.clockFace({ groups: { fun: 960 }, spent: { fun: 2640 } }).empty, false, "clock not empty")
assertEqual(kid.clockFace({ groups: { fun: 0 }, spent: { fun: 3600 } }).leftLabel, "0min LEFT", "clock empty label")
assertEqual(kid.clockFace({ groups: { fun: 0 }, spent: { fun: 3600 } }).empty, true, "clock empty")
assertEqual(kid.clockFace({ groups: { fun: 0 } }).fill, 0, "clock empty fill")
assertEqual(kid.clockFace({ groups: { fun: 1800 } }, "fun").waiting, true, "clock waiting")
assertEqual(kid.barLabel({ focused_app: "minecraft", groups: { fun: 3600 } }), "1h left", "freetime still shows remaining")
assertEqual(kid.askBlocked({ bedtime_active: true }), false, "ask open bedtime")
assertEqual(kid.askBlocked({ parent_locked: true }), false, "ask open lock")
assertEqual(kid.askBlocked({ groups: { fun: 0 } }), false, "ask open at empty")
assertEqual(kid.askBlocked({ mode: "freetime" }), false, "ask open in leftover mode")
assertEqual(kid.clockEmpty({ groups: { fun: 0 } }), true, "empty clock")
assertEqual(kid.clockEmpty({ groups: { fun: 1 } }), false, "has remaining")
assertEqual(kid.askBlocked({}), false, "ask open")
assertEqual(kid.overlayAskWaiting({ pending_ask_count: 1 }), true, "overlay waiting")
assertEqual(kid.overlayAskWaiting({ pending_ask_count: 0 }), false, "overlay not waiting")
assertEqual(kid.overlayAskWaiting({}), false, "overlay waiting missing")
assertEqual(kid.clampOverlayAskMinutes(0), 10, "overlay ask clamp min")
assertEqual(kid.clampOverlayAskMinutes(200), 120, "overlay ask clamp max")
assertEqual(kid.nudgeOverlayAskMinutes(30, -10), 20, "overlay ask nudge down")
assertEqual(kid.nudgeOverlayAskMinutes(10, -10), 10, "overlay ask nudge floor")
assertEqual(kid.nudgeOverlayAskMinutes(120, 10), 120, "overlay ask nudge ceil")
assertEqual(kid.overlayAskStepperLabel(30), "30", "overlay ask stepper")
assertEqual(kid.clampAskMinutes(0), 5, "ask clamp min")
assertEqual(kid.clampAskMinutes(200), 120, "ask clamp max")
assertEqual(kid.nudgeAskMinutes(30, -5), 25, "ask nudge down")
assertEqual(kid.nudgeAskMinutes(5, -5), 5, "ask nudge floor")
assertEqual(kid.nudgeAskMinutes(120, 5), 120, "ask nudge ceil")
assertEqual(kid.bedtimeEnd({ bedtime_end: "07:00" }), "7:00 AM", "bedtime end")
assertEqual(kid.bedtimeEnd({ bedtime_end: "07:00", hour12: false }), "07:00", "bedtime end 24h")
assertEqual(kid.parseStatus({}).hour12, true, "hour12 default")
assertEqual(kid.parseStatus({ hour12: false }).hour12, false, "hour12 off")
assertEqual(kid.bedtimeBanner({ bedtime_in: 480, bedtime_start: "21:00" }), "bedtime starts at 9:00 PM", "bedtime banner")
assertEqual(kid.bedtimeBanner({ bedtime_in: 480, bedtime_start: "21:00", hour12: false }), "bedtime starts at 21:00", "bedtime banner 24h")
assertEqual(kid.bedtimeBanner({ bedtime_in: 480, bedtime_active: true, bedtime_start: "21:00" }), "", "banner off at bedtime")
assertEqual(kid.bedtimeBanner({}), "", "banner missing")
assertEqual(kid.bedtimeBanner({ bedtime_in: 480 }), "", "banner needs start clock")
assertEqual(kid.bankSetting({ askToken: "bank" }, { askToken: "settings" }, "askToken", ""), "bank", "bank wins over plugin settings")
assertEqual(kid.bankSetting({ askToken: "" }, { askToken: "settings" }, "askToken", ""), "settings", "settings when bank empty")
assertEqual(kid.bankSetting({}, {}, "askToken", ""), "", "empty token")
assertEqual(kid.bankSetting({ askToken: "from-bar" }, {}, "askToken", ""), "from-bar", "kid-bar token")
const householdAsk = parent.parseHousehold({
  kids: [{ id: "m1", name: "Ada", asks: [{ id: "a1", group: "fun", seconds: 1800, reason: "more time", status: "pending" }] }]
})
assertEqual(parent.parseAsks(householdAsk[0].asks)[0].seconds, 1800, "household asks keep seconds")
assertEqual(parent.projectTape([{
  name: "Ada",
  id: "m1",
  claimed: false,
  reachable: true,
  status: {},
  asks: parent.parseAsks(householdAsk[0].asks)
}], 0, parent.chromeHome(), null, { pinSet: true }).bellCount, 1, "household ask reaches parent tape")
assertEqual(parent.projectTape([{
  name: "Ada",
  id: "m1",
  claimed: false,
  reachable: true,
  status: {},
  asks: parent.parseAsks(householdAsk[0].asks)
}], 0, parent.chromeHome(), null, { pinSet: true }).asks[0].text, "Ada asked for 30 more minutes", "household ask card")
assertEqual(kid.barUrgent({ parent_locked: true }), true, "urgent locked")
assertEqual(kid.barUrgent({ bedtime_active: true }), true, "urgent bedtime")
assertEqual(kid.barUrgent({
  groups: { fun: 600 }
}), true, "urgent 10m")
assertEqual(kid.barUrgent({
  groups: { fun: 601 }
}), false, "not urgent 10m1s")
assertEqual(kid.barUrgent({ bedtime_in: 600 }), true, "urgent bedtime_in 10")
assertEqual(kid.barUrgent({
  groups: { fun: 2400 }
}), false, "plenty left not urgent")
assertEqual(kid.barUrgent({
  groups: { fun: 3600 }
}), false, "full hour not urgent")
assertEqual(kid.barUrgent({ mode: "freetime", parent_locked: true }), true, "lock still urgent")

const eveningRows = kid.groupRows({
  mode: "evening",
  focused_group: "minecraft",
  piles: classicPiles,
  groups: { minecraft: 2400, youtube: 1800, fun: 0 }
})
assertEqual(eveningRows.map(r => r.name), ["Games", "YouTube", "Fun"], "evening names")
assertEqual(eveningRows.map(r => r.focused), [true, false, false], "evening focus")
assertEqual(eveningRows[2].low, true, "fun zero is low")
const waitingRows = kid.groupRows({
  mode: "evening",
  focused_group: "minecraft",
  piles: classicPiles,
  groups: { minecraft: 2400, youtube: 1800, fun: 0 },
  pending_ask_count: 1
}, "minecraft")
assertEqual(waitingRows.map(r => r.waiting), [true, false, false], "waiting spinner row")
assertEqual(kid.groupRows({
  piles: classicPiles,
  groups: { minecraft: 2400, youtube: 1800, fun: 0 }
}, "minecraft").map(r => r.waiting), [true, false, false], "spinner before poll")
assertEqual(kid.groupRows({
  piles: classicPiles,
  pending_ask_count: 1,
  groups: { minecraft: 2400, youtube: 1800, fun: 0 }
}).map(r => r.waiting), [false, false, false], "pending without local ask")
const lowRows = kid.groupRows({
  mode: "evening",
  focused_group: "minecraft",
  piles: classicPiles,
  groups: { minecraft: 180, youtube: 0, fun: 120 }
})
assertEqual(lowRows.map(r => r.low), [true, true, true], "low fills")
assertEqual(kid.panelCaption({ mode: "morning" }), "", "no morning title")
assertEqual(kid.panelCaption({ mode: "homework" }), "", "no homework title")
assertEqual(kid.barLabel({ mode: "morning", groups: { fun: 3600 } }), "1h left", "morning bar")
assertEqual(kid.pileDisplayName("youtube", { piles: classicPiles, groups: { youtube: 1 } }), "YouTube", "youtube name")
assertEqual(kid.parseStatus({
  piles: [{ id: "new", name: "Minecraft" }, { id: "new-2", name: "Fun" }, { id: "new-3", name: "Youtube" }],
  groups: { new: 0, "new-2": 2700, "new-3": 0, minecraft: 3600 }
}).groups["new-2"], 2700, "parseStatus copies groups")

assertEqual(kid.crossedWarning(901, 900), 900, "cross 15")
assertEqual(kid.crossedWarning(301, 300), 300, "cross 5")
assertEqual(kid.crossedWarning(61, 60), 60, "cross 1")
assertEqual(kid.crossedWarning(900, 899), 0, "already at 15")
assertEqual(kid.crossedWarning(901, 0), 0, "zero is the freeze")
assertEqual(kid.crossedWarning(901, 50), 60, "lag fires lowest")
assertEqual(kid.warningCopy("fun", 900), "15 min left", "remaining copy")
assertEqual(kid.warningCopy("bedtime", 60, { bedtime_start: "21:00" }), "bedtime starts at 9:00 PM", "bedtime copy")

const mcStatus = (path, bed) => ({
  focused_group: "minecraft",
  path_remaining: { minecraft: path, fun: 0 },
  bedtime_in: bed,
  bedtime_start: "21:00",
  bedtime_active: false
})
let warned = kid.takeWarnings(kid.emptyWarnState(), mcStatus(901, 901))
assertEqual(warned.notices, [], "seed silent")
warned = kid.takeWarnings(warned.state, mcStatus(900, 900))
assertEqual(warned.notices, ["15 min left", "bedtime starts at 9:00 PM"], "15 min both")
warned = kid.takeWarnings(warned.state, mcStatus(300, 300))
assertEqual(warned.notices, ["5 min left", "bedtime starts at 9:00 PM"], "5 min both")
warned = kid.takeWarnings(warned.state, mcStatus(60, 60))
assertEqual(warned.notices, ["1 min left", "bedtime starts at 9:00 PM"], "1 min both")
warned = kid.takeWarnings(warned.state, mcStatus(59, 59))
assertEqual(warned.notices, [], "no repeat")
warned = kid.takeWarnings(warned.state, {
  focused_group: "school",
  path_remaining: { minecraft: 59, fun: 0 },
  bedtime_in: 59,
  bedtime_active: false
})
assertEqual(warned.notices, [], "bypass school")
warned = kid.takeWarnings(kid.emptyWarnState(), {
  focused_group: "minecraft",
  path_remaining: { minecraft: 50, fun: 0 }
})
assertEqual(warned.notices, [], "first poll already low")
warned = kid.takeWarnings(warned.state, {
  focused_group: "minecraft",
  path_remaining: { minecraft: 49, fun: 0 }
})
assertEqual(warned.notices, [], "stay low without bedtime_in")
warned = kid.takeWarnings(warned.state, {
  focused_group: "minecraft",
  path_remaining: { minecraft: 1200, fun: 0 }
})
assertEqual(warned.notices, [], "grant raises remaining")
warned = kid.takeWarnings(warned.state, {
  focused_group: "minecraft",
  path_remaining: { minecraft: 900, fun: 0 }
})
assertEqual(warned.notices, ["15 min left"], "warn again after grant")

assertEqual(kid.parentPinLabel(), "Parent Pin", "parent pin label")
assertEqual(kid.validPin("1234"), true, "pin 4 digits")
assertEqual(kid.validPin("12a4"), false, "pin non-digit")
assertEqual(kid.validPin("123"), false, "pin short")
assertEqual(kid.overlayPinAdvance("12"), { ok: false, step: "pin", chosenMinutes: 30 }, "short pin stays")
assertEqual(kid.overlayPinAdvance("12a4"), { ok: false, step: "pin", chosenMinutes: 30 }, "bad pin stays")
assertEqual(kid.overlayPinAdvance("1234"), { ok: true, step: "minutes", chosenMinutes: 30 }, "ok pin continues")
assertEqual(kid.overlayFace({ overlay: false, parent_locked: true }), "", "no overlay")
assertEqual(kid.overlayFace({ overlay: true, parent_locked: true }), "locked", "overlay locked")
assertEqual(kid.overlayFace({ overlay: true, bedtime_active: true }), "bedtime", "overlay bedtime")
assertEqual(kid.overlayFace({ overlay: true, groups: { fun: 0 } }), "empty", "overlay empty")
assertEqual(kid.overlayStepperLabel(30), "30", "overlay stepper")
assertEqual(kid.nudgeAskMinutes(30, -5), 25, "overlay nudge")
assertEqual(kid.nudgeOverlayAskMinutes(30, 10), 40, "overlay ask nudge up")
assertEqual(kid.pinApprovePayload("1234", "ask-1"), { pin: "1234", ask_id: "ask-1" }, "pin approve payload")
assertEqual(kid.pinGrantPayload("1234", 600), { pin: "1234", seconds: 600 }, "pin grant payload")
assertEqual(kid.kidSettingsFromShell({
  bar: { layout: { right: [{ id: "kidtimer", url: "http://x:8742/", askToken: "a", readToken: "r" }] } }
}).url, "http://x:8742", "shell kid url")
assertEqual(kid.kidSettingsFromShell({
  bar: { layout: { right: [{ id: "io.github.adam-lagerhausen.kidtimer", url: "http://y:8742/", askToken: "a", readToken: "r" }] } }
}).url, "http://y:8742", "namespaced shell kid url")
assertEqual(kid.parseStatus({ parent_pin_set: true, overlay: true }).parent_pin_set, true, "parse pin set")
assertEqual(home.lockArmed, true, "fixture has household pin")
assertEqual(parent.fixtureTape("ada", parent.chromeHome(), { pinSet: true }).lockArmed, true, "household pin arms lock")
assertEqual(parent.fixtureTape("ada", parent.chromeHome(), { kidPin: true }).lockArmed, true, "kid pin arms lock")
assertEqual(parent.validPin("4242"), true, "parent pin valid")
assertEqual(parent.pinBoxText("42", 0), "4", "pin box 0")
assertEqual(parent.pinBoxText("42", 1), "2", "pin box 1")
assertEqual(parent.pinBoxText("42", 2), "", "pin box empty")
assertEqual(parent.parentPinLabel(), "Parent Pin", "parent pin label")
assertEqual(parent.parentPinWhy(), "Required for the controls. Use it to make changes on the kids computer.", "parent pin why")
assertEqual(parent.pinSlotKind("", 0, 0, false), "caret", "empty caret")
assertEqual(parent.pinSlotKind("", 1, 0, false), "empty", "empty other")
assertEqual(parent.pinSlotKind("25", 0, 2, false), "digit", "typed digit")
assertEqual(parent.pinSlotKind("25", 2, 2, false), "caret", "caret after two")
assertEqual(parent.pinSlotKind("2580", 3, 4, false), "digit", "full digit no caret")
assertEqual(parent.pinSlotKind("2580", 0, 4, true), "dot", "committed dot")

const chrome15 = parent.parseSessions([{ kind: "on", start: 12 * 60, dur: 15, label: "Chrome" }])
assertEqual(chrome15[0].start, 12 * 60, "session start minutes")
assertEqual(chrome15[0].dur, 15, "session minutes stay minutes")
assertEqual(parent.parseSessions([{ start: 8.0, dur: 0.5, label: "Chrome" }])[0].dur, 30, "fractional hours")
assertEqual(parent.parseStatus({
  today: [{ kind: "on", start: 15 * 60, dur: 1, label: "chrome" }]
}).today[0].dur, 1, "status today minutes")

const localUnix = Math.floor(new Date(2026, 8, 5, 15, 30, 0).getTime() / 1000)
assertEqual(parent.parseSessions([{ start_unix: localUnix, dur: 12, label: "chrome" }])[0].start, 15 * 60 + 30, "start_unix is parent local")
assertEqual(parent.parseSessions([{ start: 8 * 60, start_unix: localUnix, dur: 12 }])[0].start, 15 * 60 + 30, "start_unix wins over start")

assertEqual(parent.friendlyApp("foot"), "Terminal", "foot is terminal")
assertEqual(parent.friendlyApp("footclient"), "Terminal", "footclient is terminal")
assertEqual(parent.friendlyApp("google-chrome"), "Chrome", "google-chrome")
assertEqual(parent.friendlyApp("on"), "on", "on stays on")

assertEqual(parent.clockLabel(21 * 60, true), "9:00 PM", "12h bed")
assertEqual(parent.clockLabel(21 * 60, false), "21:00", "24h bed")
assertEqual(parent.clockLabel(7 * 60 + 40, true), "7:40 AM", "12h log")
assertEqual(parent.clockLabel(7 * 60 + 40, false), "07:40", "24h log")
assertEqual(parent.trackHours(true).join(" "), "12a 6a 12p 6p 12a", "12h marks")
assertEqual(parent.trackHours(false).join(" "), "0 6 12 18 24", "24h marks")
assertEqual(parent.parsePrefs("").hour12, true, "prefs empty")
assertEqual(parent.parsePrefs('{"hour12":false}').hour12, false, "prefs 24h")
assertEqual(parent.hour12Payload(false).hour12, false, "hour12 payload")

const policy = parent.defaultPolicy()
const endsAtNow = parent.layoutTrack([
  { kind: "on", start: 16 * 60 + 39, dur: 3, label: "chrome" }
], policy, 16 + 42 / 60, true)
assertEqual(endsAtNow.blocks.length, 1, "three minutes at now stays visible")
assertEqual(Math.round(endsAtNow.blocks[0].widthPct), Math.round(3 / 1440 * 100), "three minutes wide")
assertEqual(endsAtNow.blocks[0].leftPct + endsAtNow.blocks[0].widthPct <= endsAtNow.needle + 1e-9, true, "three minutes does not pass needle")

const pastNeedle = parent.layoutTrack([
  { kind: "on", start: 16 * 60, dur: 90, label: "chrome" }
], policy, 16.5, true)
const pastEnd = pastNeedle.blocks[0].leftPct + pastNeedle.blocks[0].widthPct
assertEqual(pastEnd <= pastNeedle.needle + 1e-9, true, "long sit clipped to now")
assertEqual(Math.round(pastNeedle.blocks[0].widthPct), Math.round(30 / 1440 * 100), "clipped width is elapsed")

const footFlicker = parent.layoutTrack([
  { kind: "on", start: 16 * 60 + 59, dur: 16, label: "foot" },
  { kind: "on", start: 17 * 60 + 14, dur: 1, label: "foot" },
  { kind: "on", start: 17 * 60 + 14, dur: 1, label: "foot" },
  { kind: "on", start: 17 * 60 + 14, dur: 60, label: "foot" },
  { kind: "on", start: 18 * 60 + 14, dur: 1, label: "foot" },
  { kind: "on", start: 18 * 60 + 14, dur: 1, label: "foot" }
], policy, 17.25, true)
assertEqual(footFlicker.log.length, 1, "foot flicker is one sitting")
assertEqual(footFlicker.log[0].clock, "4:59 PM", "foot sitting clock")
assertEqual(footFlicker.log[0].name, "TERMINAL", "foot sitting name")
assertEqual(footFlicker.log[0].dur, "1h 16m", "foot sitting wall clock")
assertEqual(footFlicker.blocks.length, 1, "foot occupancy merges")
assertEqual(Math.round(footFlicker.blocks[0].widthPct), Math.round(16 / 1440 * 100), "foot occupancy clipped to now")
const flickerEnd = footFlicker.blocks[0].leftPct + footFlicker.blocks[0].widthPct
assertEqual(flickerEnd <= footFlicker.needle + 1e-9, true, "foot occupancy stays left of needle")

const overlap = parent.layoutTrack([
  { kind: "on", start: 17 * 60 + 14, dur: 1, label: "foot" },
  { kind: "on", start: 17 * 60 + 14, dur: 60, label: "foot" }
], policy, 18.5, true)
assertEqual(overlap.log[0].dur, "1h", "overlapping durs do not stack")
assertEqual(overlap.blocks.length, 1, "overlap one block")

const mixed = parent.layoutTrack([
  { kind: "on", start: 15 * 60, dur: 20, label: "minecraft" },
  { kind: "on", start: 15 * 60 + 20, dur: 12, label: "chrome" }
], policy, 16, true)
assertEqual(mixed.log.length, 1, "mixed one sitting")
assertEqual(mixed.log[0].name, "MINECRAFT + CHROME", "mixed names longest first")
assertEqual(mixed.log[0].dur, "32m", "mixed wall clock")

const dust = parent.layoutTrack([
  { kind: "on", start: 15 * 60, dur: 20, label: "minecraft" },
  { kind: "on", start: 15 * 60 + 20, dur: 1, label: "chrome" }
], policy, 16, true)
assertEqual(dust.log[0].name, "MINECRAFT", "dust chrome omitted")

const seven = []
for (let i = 0; i < 7; i++) {
  seven.push({ kind: "on", start: (8 + i) * 60, dur: 10, label: "chrome" })
}
const capped = parent.layoutTrack(seven, policy, 15, true)
assertEqual(capped.log.length, 6, "seven sittings cap at six")
assertEqual(capped.log[0].name, "EARLIER", "earlier row")
assertEqual(capped.log[0].clock, "8:00 AM", "earlier clock is oldest folded")
assertEqual(capped.log[0].dur, "20m", "earlier sums folded sitting lengths")
assertEqual(capped.log[1].clock, "10:00 AM", "five newest start")
assertEqual(capped.log[5].clock, "2:00 PM", "newest sitting kept")
assertEqual(capped.blocks.length, 7, "track keeps every sit")
const capped24 = parent.layoutTrack(seven, policy, 15, true, false)
assertEqual(capped24.log[0].clock, "08:00", "24h earlier")
assertEqual(capped24.log[5].clock, "14:00", "24h newest")

const six = seven.slice(0, 6)
const sixLog = parent.layoutTrack(six, policy, 14, true)
assertEqual(sixLog.log.length, 6, "six sittings stay six")
assertEqual(sixLog.log[0].name, "CHROME", "six has no earlier")

console.log("ok")
