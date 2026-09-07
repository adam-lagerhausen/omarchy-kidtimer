import QtQuick
import Quickshell
import Quickshell.Io
import qs.Ui
import "KidModel.js" as Model

BarWidget {
  id: root
  moduleName: "kidtimer.kid"

  property string statusText: "kidtimer"
  property bool urgentChip: false
  property var statusJson: ({})
  property var lookPiles: []
  property var warnState: Model.emptyWarnState()

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
    target.bar = root.bar
    target.settings = root.settings
    target.anchorItem = button
    target.hostWidget = root
    target.statusJson = root.statusJson
    target.lookPiles = root.lookPiles
  }

  function notify(body) {
    Quickshell.execDetached([
      "omarchy-notification-send",
      "--app-name", "Kidtimer",
      "Kidtimer",
      body
    ])
  }

  function bankUrl() {
    return String(setting("url", "http://127.0.0.1:8742")).replace(/\/$/, "")
  }

  function poll() {
    var req = new XMLHttpRequest()
    req.open("GET", bankUrl() + "/v1/status")
    req.setRequestHeader("Authorization", "Bearer " + String(setting("readToken", "")))
    req.onreadystatechange = function() {
      if (req.readyState !== XMLHttpRequest.DONE || req.status !== 200) return
      var next
      try {
        next = Model.parseStatus(JSON.parse(req.responseText))
      } catch (e) {
        return
      }
      lookPiles = next.piles
      statusJson = next
      statusText = Model.barLabel(next)
      urgentChip = Model.barUrgent(next)
      var warned = Model.takeWarnings(warnState, next)
      warnState = warned.state
      for (var i = 0; i < warned.notices.length; i++) {
        notify(warned.notices[i])
      }
    }
    try {
      req.send()
    } catch (e) {
      return
    }
  }

  implicitWidth: button.implicitWidth
  implicitHeight: button.implicitHeight

  onBarChanged: injectPanel()
  onSettingsChanged: injectPanel()
  onStatusJsonChanged: injectPanel()
  onLookPilesChanged: injectPanel()

  IpcHandler {
    target: "kidtimer.kid"
    function open(): void { root.open() }
    function close(): void { root.close() }
    function show(): void { root.open() }
    function hide(): void { root.close() }
    function toggle(): void { root.togglePanel() }
  }

  Timer {
    interval: 1000
    running: true
    repeat: true
    triggeredOnStart: true
    onTriggered: poll()
  }

  Process {
    id: kidSync
    running: true
    command: [Model.kidtimerBin(root.settings, Quickshell.env("HOME"), Qt.resolvedUrl("bin/kidtimer").toString().replace(/^file:\/\//, "")), "kid"]
    onExited: kidRestart.restart()
  }

  Timer {
    id: kidRestart
    interval: 2000
    repeat: false
    onTriggered: kidSync.running = true
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
    active: root.urgentChip
    tooltipText: ""
    onPressed: function(b) {
      if (b === Qt.RightButton) return
      root.togglePanel()
    }
  }
}
