import QtQuick
import qs.Commons
import qs.Ui
import "ParentModel.js" as Model

Item {
  id: root

  property var bar: null
  property var anchorItem: null
  property var hostWidget: null
  property var snapshots: []
  property int selectedIndex: 0
  property bool holdLook: false
  property var chrome: Model.chromeHome()
  property var nowPtr: new Date()
  property bool householdPinSet: false
  property bool hour12: true

  readonly property var barIdentity: hostWidget || root
  readonly property var tape: Model.projectTape(snapshots, selectedIndex, chrome, nowPtr, { pinSet: householdPinSet, hour12: root.hour12 })
  readonly property real panelWidth: 340
  width: parent ? parent.width : panelWidth
  implicitHeight: paper.implicitHeight

  function setPage(id) {
    if (id === "settings") root.chrome = Model.chromeSettings()
    else root.chrome = Model.chromeHome()
  }

  function fire(ev) {
    var r = Model.reduceChrome(root.chrome, ev, root.tape)
    root.chrome = r.chrome
    if (r.selectIndex !== undefined) {
      root.selectedIndex = r.selectIndex
      if (root.hostWidget) root.hostWidget.selectedIndex = r.selectIndex
    }
    if (ev.kind === "clock") root.hour12 = ev.hour12 !== false
    if (!root.hostWidget) return
    if (ev.kind === "clock") root.hostWidget.setHour12(root.hour12)
    if (ev.kind === "lock") {
      if (!root.tape.lockArmed) {
        root.chrome = Model.chromeSettings()
      } else if (root.tape.ours !== false) {
        root.hostWidget.setLock(!root.tape.kid.locked)
      }
    }
    if (ev.kind === "pinSet" && ev.pin) root.hostWidget.setParentPin(ev.pin)
    if (ev.kind === "minus10" && root.tape.ours !== false) root.hostWidget.grantFun(-600)
    if (ev.kind === "plus10" && root.tape.ours !== false) root.hostWidget.grantFun(600)
    if (ev.kind === "deny" && ev.ask) root.hostWidget.denyAsk(ev.ask)
    if (ev.kind === "approve" && ev.ask) root.hostWidget.approveAsk(ev.ask)
    if (r.adopt) {
      var adoptSnap = (root.snapshots || [])[r.adopt.index]
      if (adoptSnap && root.hostWidget && root.hostWidget.adoptKid) root.hostWidget.adoptKid(adoptSnap)
    }
    if (ev.kind === "bed" || ev.kind === "up" || ev.kind === "funDay" || ev.kind === "addThing" || ev.kind === "removeThing") {
      var snap = (root.snapshots || [])[root.selectedIndex]
      if (!snap) return
      var next = Model.applyPolicy(snap, ev)
      root.hostWidget.persistPolicy(next)
    }
    if (ev.kind === "query" && root.hostWidget && root.hostWidget.searchList) {
      root.hostWidget.searchList(ev.list, ev.q)
    }
  }

  onSelectedIndexChanged: {
    var ch = root.chrome
    root.chrome = {
      face: ch.face,
      picker: false,
      bell: ch.bell,
      adopt: ch.adopt || null,
      query: ch.query || { fun: "", school: "" },
      hits: ch.hits || { fun: [], school: [] }
    }
  }

  onOpenedChanged: {
    if (root.opened) root.nowPtr = new Date()
  }

  Timer {
    interval: 60000
    running: hostWidget && hostWidget.opened
    repeat: true
    triggeredOnStart: true
    onTriggered: root.nowPtr = new Date()
  }

  Column {
    width: parent.width
    Tape {
      id: paper
      width: parent.width
      tape: root.tape
      bar: root.bar
      onAct: function (ev) { root.fire(ev) }
    }
    LookBtn {
      width: parent.width - 32
      x: 16
      text: "This is the kid's computer"
      ghost: true
      fontFamily: paper.plex
      onClicked: if (root.hostWidget) root.hostWidget.pickRole("kid")
    }
  }
}
