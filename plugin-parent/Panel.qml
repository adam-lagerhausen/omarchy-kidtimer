import QtQuick
import qs.Commons
import qs.Ui
import "ParentModel.js" as Model

Panel {
  id: root
  moduleName: "kidtimer.parent"
  ipcTarget: "kidtimer.parent"
  manageIpc: false

  property var anchorItem: null
  property var hostWidget: null
  property var snapshots: []
  property int selectedIndex: 0
  property bool holdLook: false
  property var chrome: Model.chromeHome()
  property var nowPtr: new Date()
  property bool householdPinSet: false

  readonly property var barIdentity: hostWidget || root
  readonly property var tape: Model.projectTape(snapshots, selectedIndex, chrome, nowPtr, { pinSet: householdPinSet })
  readonly property real panelWidth: 340

  function open() {
    root.controller.show()
    Qt.callLater(function() {
      if (root.opened) setCenterHoverRevealSuppressed(true)
    })
  }

  function close() {
    setCenterHoverRevealSuppressed(false)
    root.controller.hide()
  }

  function toggle() {
    if (root.opened) root.close()
    else root.open()
  }

  function switchPanel(direction) {
    if (root.bar && typeof root.bar.switchPanelFrom === "function")
      return root.bar.switchPanelFrom(root.barIdentity, direction)
    return false
  }

  function setCenterHoverRevealSuppressed(value) {
    if (root.bar && "centerHoverRevealSuppressed" in root.bar)
      root.bar.centerHoverRevealSuppressed = value
  }

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
    if (!root.hostWidget) return
    if (ev.kind === "lock") {
      if (!root.tape.lockArmed) {
        root.chrome = Model.chromeSettings()
      } else {
        root.hostWidget.setLock(!root.tape.kid.locked)
      }
    }
    if (ev.kind === "pinSet" && ev.pin) root.hostWidget.setParentPin(ev.pin)
    if (ev.kind === "minus10") root.hostWidget.grantFun(-600)
    if (ev.kind === "plus10") root.hostWidget.grantFun(600)
    if (ev.kind === "deny" && ev.ask) root.hostWidget.denyAsk(ev.ask)
    if (ev.kind === "approve" && ev.ask) root.hostWidget.approveAsk(ev.ask)
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
      query: ch.query || { fun: "", school: "" },
      hits: ch.hits || { fun: [], school: [] }
    }
  }

  onOpenedChanged: {
    if (root.opened) root.nowPtr = new Date()
  }

  Timer {
    interval: 60000
    running: root.opened
    repeat: true
    triggeredOnStart: true
    onTriggered: root.nowPtr = new Date()
  }

  KeyboardPanel {
    id: panel
    anchorItem: root.anchorItem
    owner: root.barIdentity
    bar: root.bar
    open: root.opened
    centerOnBar: false
    padding: 0
    focusTarget: keyCatcher
    contentWidth: panel.fittedContentWidth(
      root.panelWidth
      + Border.left(panel.borderSpec)
      + Border.right(panel.borderSpec)
    )
    contentHeight: panel.fittedContentHeight(Math.max(1, paper.implicitHeight))

    PanelKeyCatcher {
      id: keyCatcher
      anchors.fill: parent
      blocked: paper.searchFocused
      onCloseRequested: root.close()
      onTabRequested: function(direction) { root.switchPanel(direction) }

      Flickable {
        id: flick
        anchors.fill: parent
        contentWidth: width
        contentHeight: paper.implicitHeight
        clip: true
        boundsBehavior: Flickable.StopAtBounds
        interactive: paper.implicitHeight > height
        Tape {
          id: paper
          width: parent.width
          tape: root.tape
          bar: root.bar
          onAct: function (ev) { root.fire(ev) }
        }
      }
    }
  }
}
