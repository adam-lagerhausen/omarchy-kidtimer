import QtQuick
import Quickshell
import Quickshell.Io
import qs.Ui
import "ParentModel.js" as Model

BarWidget {
  id: root
  moduleName: "kidtimer.parent"

  property string statusText: "kids"
  property var snapshots: []
  property int selectedIndex: 0
  property var seenAskIds: ({})
  property var asksSeeded: ({})
  property string searchListId: "fun"
  property string searchQ: ""
  property bool householdPinSet: false
  property bool hour12: true
  property bool prefsReady: false
  property var clockPushed: ({})


  readonly property bool opened: panelLoader.item ? panelLoader.item.opened === true : false
  readonly property bool popoutSwitchClosing: panelLoader.item ? panelLoader.item.popoutSwitchClosing === true : false
  readonly property real openPanelIndicatorWidth: button.labelWidth

  function open() {
    if (panelLoader.item) panelLoader.item.open()
  }

  function close() {
    if (panelLoader.item) panelLoader.item.close()
  }

  function togglePanel() {
    if (panelLoader.item) panelLoader.item.toggle()
  }

  function closeForPopoutSwitch() {
    if (panelLoader.item) panelLoader.item.closeForPopoutSwitch()
  }

  function injectPanel() {
    var target = panelLoader.item
    if (!target) return
    if ("bar" in target) target.bar = root.bar
    if ("settings" in target) target.settings = root.settings
    if ("anchorItem" in target) target.anchorItem = button
    if ("hostWidget" in target) target.hostWidget = root
    if ("householdPinSet" in target) target.householdPinSet = root.householdPinSet
    if ("hour12" in target) target.hour12 = root.hour12
    syncPanel()
  }

  function syncPanel() {
    var target = panelLoader.item
    if (!target) return
    if ("snapshots" in target) target.snapshots = snapshots
    if ("selectedIndex" in target) target.selectedIndex = selectedIndex
    if ("householdPinSet" in target) target.householdPinSet = root.householdPinSet
    if ("hour12" in target) target.hour12 = root.hour12
  }

  property var discoveredKids: []

  function kidRows() {
    return Model.mergeKids(root.settings || {}, discoveredKids)
  }

  function loadHousehold(raw) {
    discoveredKids = Model.parseHousehold(raw)
    seedFromSettings()
  }

  function seedFromSettings() {
    var rows = kidRows()
    var prev = snapshots || []
    var next = []
    for (var i = 0; i < rows.length; i++) {
      var cur = prev[i] || {}
      next.push({
        name: rows[i].name,
        id: rows[i].id || "",
        url: rows[i].url,
        token: rows[i].token,
        claimed: !!rows[i].claimed,
        status: cur.status || {},
        asks: cur.asks || { asks: [] },
        reachable: cur.reachable === true || !!rows[i].claimed,
        error: cur.error === true,
        look: cur.look || null
      })
    }
    snapshots = next
    statusText = Model.householdBarLabel(next)
    syncPanel()
    root.pushHour12()
  }

  function aimedKid() {
    var list = snapshots || []
    var i = selectedIndex
    if (i < 0 || i >= list.length) i = 0
    if (list[i]) return list[i]
    var rows = kidRows()
    return rows[0] || { name: "", url: "", token: "" }
  }

  function holdLook() {
    return panelLoader.item && panelLoader.item.holdLook === true
  }

  function writeSnapshot(index, row, patch) {
    var next = (snapshots || []).slice()
    var cur = next[index] || {
      name: row.name,
      url: row.url,
      token: row.token,
      status: {},
      asks: { asks: [] },
      reachable: false,
      error: false,
      look: null
    }
    var look = patch.look !== undefined ? patch.look : cur.look
    var hold = patch.hold !== undefined ? patch.hold : !!cur.hold
    if (patch.fromPoll && (root.holdLook() || hold)) look = cur.look
    next[index] = {
      name: row.name,
      id: row.id || cur.id || "",
      url: row.url,
      token: row.token,
      claimed: !!(row.claimed || cur.claimed),
      status: patch.status !== undefined ? patch.status : cur.status,
      asks: patch.asks !== undefined ? patch.asks : cur.asks,
      reachable: patch.reachable !== undefined ? patch.reachable : !!cur.reachable,
      error: patch.error !== undefined ? !!patch.error : !!cur.error,
      look: look,
      hold: hold
    }
    snapshots = next
    statusText = Model.householdBarLabel(next)
    syncPanel()
  }

  function adoptKid(row) {
    if (!row) return
    deskPost("/v1/adopt", { id: row.id || "", url: row.url || "" })
  }

  function deskUrl() {
    return String(setting("desk", "http://127.0.0.1:8741")).replace(/\/$/, "")
  }

  function applyDesk(doc) {
    discoveredKids = Model.parseHousehold(doc)
    var rows = kidRows()
    var prev = snapshots || []
    var next = []
    for (var i = 0; i < rows.length; i++) {
      var row = rows[i]
      var cur = prev[i] || {}
      if (row.claimed) {
        next.push({
          id: row.id,
          name: row.name,
          url: row.url || "",
          token: "",
          claimed: true,
          status: {},
          asks: [],
          look: null,
          reachable: true,
          error: false
        })
        continue
      }
      if (row.id && !row.url) {
        next.push({
          id: row.id,
          name: row.name,
          url: "",
          token: "",
          claimed: false,
          status: row.status && row.status.groups ? Model.parseStatus(row.status) : (cur.status || {}),
          asks: row.asks ? Model.parseAsks(row.asks) : (cur.asks || []),
          look: row.look ? Model.parseLook(row.look) : cur.look,
          reachable: row.live === true,
          error: false
        })
        continue
      }
      next.push({
        name: row.name,
        id: row.id || "",
        url: row.url,
        token: row.token,
        claimed: false,
        status: cur.status || {},
        asks: cur.asks || { asks: [] },
        reachable: cur.reachable === true,
        error: cur.error === true,
        look: cur.look || null
      })
    }
    snapshots = next
    statusText = Model.householdBarLabel(next)
    syncPanel()
    for (var j = 0; j < rows.length; j++) {
      if (rows[j].claimed) continue
      if (rows[j].url) {
        pollStatus(j, rows[j])
        pollAsks(j, rows[j])
      }
    }
    root.pushHour12()
  }

  function poll() {
    var req = new XMLHttpRequest()
    req.open("GET", deskUrl() + "/v1/household")
    req.onreadystatechange = function () {
      if (req.readyState !== XMLHttpRequest.DONE) return
      if (req.status !== 200) {
        kidsFile.reload()
        seedFromSettings()
        var rows = kidRows()
        for (var i = 0; i < rows.length; i++) {
          if (rows[i].claimed) continue
          if (rows[i].url) {
            pollStatus(i, rows[i])
            pollAsks(i, rows[i])
          }
        }
        return
      }
      try {
        applyDesk(JSON.parse(req.responseText))
      } catch (e) {
        kidsFile.reload()
        seedFromSettings()
      }
    }
    try {
      req.send()
    } catch (e) {
      kidsFile.reload()
      seedFromSettings()
    }
  }

  function pollStatus(index, row) {
    var req = new XMLHttpRequest()
    req.open("GET", row.url + "/v1/status")
    req.setRequestHeader("Authorization", "Bearer " + row.token)
    req.onreadystatechange = function() {
      if (req.readyState !== XMLHttpRequest.DONE) return
      if (req.status === 0) {
        writeSnapshot(index, row, { reachable: false, error: false })
        return
      }
      if (req.status !== 200) {
        writeSnapshot(index, row, { reachable: false, error: true })
        return
      }
      try {
        writeSnapshot(index, row, { status: Model.parseStatus(JSON.parse(req.responseText)), reachable: true, error: false })
        pollLook(index, row)
      } catch (e) {
        writeSnapshot(index, row, { reachable: false, error: true })
      }
    }
    try {
      req.send()
    } catch (e) {
      writeSnapshot(index, row, { reachable: false, error: false })
    }
  }

  function pollLook(index, row) {
    var req = new XMLHttpRequest()
    req.open("GET", row.url + "/v1/look")
    req.setRequestHeader("Authorization", "Bearer " + row.token)
    req.onreadystatechange = function() {
      if (req.readyState !== XMLHttpRequest.DONE) return
      if (req.status !== 200) return
      try {
        writeSnapshot(index, row, { look: Model.parseLook(JSON.parse(req.responseText)), fromPoll: true })
      } catch (e) {
        return
      }
    }
    try {
      req.send()
    } catch (e) {
      return
    }
  }

  function pollAsks(index, row) {
    var req = new XMLHttpRequest()
    req.open("GET", row.url + "/v1/asks")
    req.setRequestHeader("Authorization", "Bearer " + row.token)
    req.onreadystatechange = function() {
      if (req.readyState !== XMLHttpRequest.DONE) return
      if (req.status !== 200) return
      var payload
      try {
        payload = JSON.parse(req.responseText)
      } catch (e) {
        return
      }
      writeSnapshot(index, row, { asks: Model.parseAsks(payload) })
      var key = row.url
      var seeded = asksSeeded[key]
      if (!seeded) {
        var first = Object.assign({}, seenAskIds)
        first[key] = Model.pendingIds(payload)
        seenAskIds = first
        var flags = Object.assign({}, asksSeeded)
        flags[key] = true
        asksSeeded = flags
        return
      }
      var prev = seenAskIds[key] || []
      var fresh = Model.newAskIds(prev, payload)
      var nextSeen = Object.assign({}, seenAskIds)
      nextSeen[key] = Model.pendingIds(payload)
      seenAskIds = nextSeen
      for (var i = 0; i < fresh.length; i++) {
        notifyAsk(Model.findAsk(payload, fresh[i]), snapshots[index] || row)
      }
    }
    try {
      req.send()
    } catch (e) {
      return
    }
  }

  function notifyAsk(ask, kid) {
    Quickshell.execDetached([
      "omarchy-notification-send",
      "--app-name", "Kidtimer",
      Model.notifyHeadline(kid && kid.name),
      Model.notifySummary(ask, kid && kid.look, kid && kid.status)
    ])
  }

  function postJsonTo(kid, method, path, body, thenFn) {
    if (!kid || !kid.url) return
    var req = new XMLHttpRequest()
    req.open(method, kid.url + path)
    req.setRequestHeader("Authorization", "Bearer " + kid.token)
    req.setRequestHeader("Content-Type", "application/json")
    if (path === "/v1/grants") {
      req.setRequestHeader("Idempotency-Key", "parent-" + Date.now() + "-" + Math.floor(Math.random() * 1e9))
    }
    req.onreadystatechange = function() {
      if (req.readyState !== XMLHttpRequest.DONE) return
      if (req.status === 200) {
        if (path === "/v1/look") {
          try {
            var parsed = Model.parseLook(JSON.parse(req.responseText))
            var rows = kidRows()
            var idx = root.selectedIndex
            if (rows[idx]) writeSnapshot(idx, rows[idx], { look: parsed })
          } catch (e) {
          }
        }
        poll()
        if (thenFn) thenFn()
      }
    }
    try {
      req.send(JSON.stringify(body))
    } catch (e) {
      return
    }
  }

  function postJson(method, path, body, thenFn) {
    postJsonTo(aimedKid(), method, path, body, thenFn)
  }

  function grant(group, seconds) {
    grantFun(seconds)
  }

  function deskSend(method, path, body, thenFn) {
    var req = new XMLHttpRequest()
    req.open(method, deskUrl() + path)
    req.setRequestHeader("Content-Type", "application/json")
    if (path.indexOf("/grants") >= 0) {
      req.setRequestHeader("Idempotency-Key", "parent-" + Date.now() + "-" + Math.floor(Math.random() * 1e9))
    }
    req.onreadystatechange = function () {
      if (req.readyState !== XMLHttpRequest.DONE) return
      if (req.status === 200) {
        poll()
        if (thenFn) thenFn()
      }
    }
    try {
      req.send(JSON.stringify(body))
    } catch (e) {
      return
    }
  }

  function deskPost(path, body, thenFn) {
    deskSend("POST", path, body, thenFn)
  }

  function sendKid(kid, method, path, body, thenFn) {
    if (kid && kid.id && !kid.url) {
      var rest = String(path || "")
      if (rest.indexOf("/v1/") === 0) rest = rest.slice(3)
      deskSend(method, "/v1/kids/" + kid.id + rest, body, thenFn)
      return
    }
    postJsonTo(kid, method, path, body, thenFn)
  }

  function grantFun(seconds) {
    var kid = aimedKid()
    if (kid && kid.claimed) return
    if (kid && kid.id && !kid.url) {
      deskPost("/v1/kids/" + kid.id + "/grants", Model.grantPayload("fun", seconds))
      return
    }
    postJson("POST", "/v1/grants", Model.grantPayload("fun", seconds))
  }

  function decide(askId, decision, kidIndex) {
    var list = snapshots || []
    var kid = aimedKid()
    if (kidIndex !== undefined && kidIndex !== null && list[kidIndex]) kid = list[kidIndex]
    if (kid && kid.id && !kid.url) {
      deskPost("/v1/kids/" + kid.id + "/asks/" + askId + "/decide", { decision: decision })
      return
    }
    postJsonTo(kid, "POST", "/v1/asks/" + askId + "/decide", { decision: decision })
  }

  function denyAsk(ask) {
    if (!ask) return
    decide(ask.id, "deny", ask.kidIndex)
  }

  function approveAsk(ask) {
    if (!ask) return
    decide(ask.id, "approve", ask.kidIndex)
  }

  function setLock(locked) {
    var kid = aimedKid()
    if (kid && kid.claimed) return
    if (kid && kid.id && !kid.url) {
      deskPost("/v1/kids/" + kid.id + "/lock", Model.lockPayload(locked))
      return
    }
    postJson("POST", "/v1/lock", Model.lockPayload(locked))
  }

  function setParentPin(digits) {
    if (!Model.validPin(digits)) return
    pinSetProc.secret = String(digits)
    pinSetProc.running = true
  }

  function setHour12(on) {
    var next = on !== false
    if (root.hour12 !== next) root.clockPushed = ({})
    root.hour12 = next
    prefsFile.setText(Model.prefsWire(root.hour12))
    if (panelLoader.item && "hour12" in panelLoader.item) panelLoader.item.hour12 = root.hour12
    root.pushHour12()
  }

  function loadPrefs(raw) {
    var p = Model.parsePrefs(raw)
    if (root.hour12 !== p.hour12) root.clockPushed = ({})
    root.hour12 = p.hour12
    root.prefsReady = true
    if (panelLoader.item && "hour12" in panelLoader.item) panelLoader.item.hour12 = root.hour12
    root.pushHour12()
  }

  function pushHour12() {
    if (!root.prefsReady) return
    var body = Model.hour12Payload(root.hour12)
    var rows = kidRows()
    var pushed = {}
    for (var k in root.clockPushed) pushed[k] = root.clockPushed[k]
    for (var i = 0; i < rows.length; i++) {
      var row = rows[i]
      if (!row || row.claimed) continue
      var id = row.id || row.url || ""
      if (!id) continue
      if (pushed[id]) continue
      sendKid(row, "PATCH", "/v1/policy", body)
      pushed[id] = true
    }
    root.clockPushed = pushed
  }

  function setBedtime(start, end) {
    sendKid(aimedKid(), "PATCH", "/v1/policy", Model.bedtimePayload(start, end))
  }

  function putLook(look, thenFn) {
    var kid = aimedKid()
    var rows = kidRows()
    var idx = root.selectedIndex
    if (rows[idx]) writeSnapshot(idx, rows[idx], { look: look, hold: true })
    sendKid(kid, "PUT", "/v1/look", Model.lookToWire(look), function () {
      if (rows[idx]) writeSnapshot(idx, rows[idx], { hold: false })
      if (thenFn) thenFn()
    })
  }

  function persistPolicy(look) {
    var kid = aimedKid()
    var rows = kidRows()
    var idx = root.selectedIndex
    var p = look && look.policy
    var patch = { look: look, hold: true }
    if (p) {
      var st = Model.clone(Model.snapshotStatus((snapshots || [])[idx] || {}))
      st.bedtimeStart = p.bed
      st.bedtimeEnd = p.up
      patch.status = st
    }
    if (rows[idx]) writeSnapshot(idx, rows[idx], patch)
    if (p) sendKid(kid, "PATCH", "/v1/policy", Model.bedtimePayload(p.bed, p.up))
    sendKid(kid, "PUT", "/v1/look", Model.lookToWire(look), function () {
      if (rows[idx]) writeSnapshot(idx, rows[idx], { hold: false })
    })
  }

  function patchPolicy(body) {
    sendKid(aimedKid(), "PATCH", "/v1/policy", body)
  }

  function searchList(list, q) {
  }

  implicitWidth: button.implicitWidth
  implicitHeight: button.implicitHeight

  onBarChanged: injectPanel()
  onSettingsChanged: {
    seedFromSettings()
    injectPanel()
  }
  onSelectedIndexChanged: syncPanel()
  Component.onCompleted: seedFromSettings()

  IpcHandler {
    target: "kidtimer.parent"
    function open(): void { root.open() }
    function close(): void { root.close() }
    function show(): void { root.open() }
    function hide(): void { root.close() }
    function toggle(): void { root.togglePanel() }
    function page(id: string): void {
      root.open()
      if (panelLoader.item && typeof panelLoader.item.setPage === "function")
        panelLoader.item.setPage(id)
    }
  }

  Timer {
    interval: 5000
    running: true
    repeat: true
    triggeredOnStart: true
    onTriggered: poll()
  }

  FileView {
    id: kidsFile
    path: {
      var p = String(setting("kidsFile", ""))
      if (p) return p
      return Quickshell.env("HOME") + "/.local/share/kidtimer/kids.json"
    }
    watchChanges: true
    printErrors: false
    onLoaded: root.loadHousehold(text())
    onFileChanged: kidsFile.reload()
    onLoadFailed: root.loadHousehold("")
  }

  FileView {
    id: prefsFile
    path: {
      var p = String(setting("prefsFile", ""))
      if (p) return p
      return Quickshell.env("HOME") + "/.local/share/kidtimer/prefs.json"
    }
    watchChanges: true
    printErrors: false
    onLoaded: root.loadPrefs(text())
    onFileChanged: prefsFile.reload()
    onLoadFailed: root.loadPrefs("")
  }

  Process {
    id: parentSync
    running: true
    command: [Model.kidtimerBin(root.settings, Quickshell.env("HOME"), Qt.resolvedUrl("bin/kidtimer").toString().replace(/^file:\/\//, "")), "parent"]
    onExited: parentRestart.restart()
  }

  Process {
    id: pinSetProc
    property string secret: ""
    stdinEnabled: true
    command: [Model.parentBin(root.settings, Quickshell.env("HOME")), "pin", "set"]
    onStarted: {
      write(secret + "\n")
      secret = ""
    }
    onExited: pinStat.running = true
  }

  Process {
    id: pinStat
    running: true
    command: ["test", "-f", Quickshell.env("HOME") + "/.local/share/kidtimer/parent-pin"]
    onExited: {
      root.householdPinSet = exitCode === 0
      root.injectPanel()
    }
  }

  Timer {
    interval: 5000
    running: true
    repeat: true
    onTriggered: pinStat.running = true
  }

  Timer {
    id: parentRestart
    interval: 2000
    repeat: false
    onTriggered: parentSync.running = true
  }

  Loader {
    id: panelLoader
    active: true
    source: Qt.resolvedUrl("Panel.qml")
    visible: false
    onLoaded: {
      root.injectPanel()
      Qt.callLater(root.injectPanel)
    }
  }

  WidgetButton {
    id: button
    anchors.fill: parent
    bar: root.bar
    text: root.statusText
    tooltipText: ""
    onPressed: function(b) {
      if (b === Qt.RightButton) {
        root.grantFun(600)
        return
      }
      root.togglePanel()
    }
  }
}
