import QtQuick
import qs.Commons
import qs.Ui

Panel {
  id: root
  moduleName: "kidtimer"
  ipcTarget: "kidtimer"
  manageIpc: false

  property var anchorItem: null
  property var hostWidget: null
  property string role: ""
  property var snapshots: []
  property int selectedIndex: 0
  property bool holdLook: false
  property var chrome: ({})
  property var nowPtr: new Date()
  property bool householdPinSet: false
  property bool hour12: true
  property var statusJson: ({})
  property var lookPiles: []
  property bool busy: false
  readonly property bool setupBusy: busy || (hostWidget ? hostWidget.setupBusy === true : false)
  readonly property color ink: bar ? bar.foreground : Color.popups.text
  FontLoader { id: plexReg; source: Qt.resolvedUrl("fonts/JetBrainsMono-Regular.ttf") }
  readonly property string plex: {
    if (hostWidget && hostWidget.plex) return hostWidget.plex
    return plexReg.status === FontLoader.Ready ? plexReg.name : "JetBrains Mono"
  }

  readonly property var barIdentity: hostWidget || root
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
    if (faceLoader.item && typeof faceLoader.item.setPage === "function")
      faceLoader.item.setPage(id)
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
    contentHeight: panel.fittedContentHeight(Math.max(1, faceLoader.item ? faceLoader.item.implicitHeight : 120))

    PanelKeyCatcher {
      id: keyCatcher
      anchors.fill: parent
      onCloseRequested: root.close()
      onTabRequested: function(direction) { root.switchPanel(direction) }

      Loader {
        id: faceLoader
        width: parent.width
        source: {
          if (root.role === "parent") return Qt.resolvedUrl("ParentPanel.qml")
          if (root.role === "kid") return Qt.resolvedUrl("KidPanel.qml")
          return Qt.resolvedUrl("Setup.qml")
        }
        onLoaded: {
          var t = item
          if (!t) return
          if ("bar" in t) t.bar = root.bar
          if ("hostWidget" in t) t.hostWidget = root.hostWidget
          if ("anchorItem" in t) t.anchorItem = root.anchorItem
          if ("snapshots" in t) t.snapshots = root.snapshots
          if ("selectedIndex" in t) t.selectedIndex = root.selectedIndex
          if ("householdPinSet" in t) t.householdPinSet = root.householdPinSet
          if ("hour12" in t) t.hour12 = root.hour12
          if ("statusJson" in t) t.statusJson = root.statusJson
          if ("lookPiles" in t) t.lookPiles = root.lookPiles
          if ("chrome" in t && root.chrome && root.chrome.face) t.chrome = root.chrome
        }
      }

      Rectangle {
        id: setupWait
        anchors.fill: parent
        visible: root.setupBusy
        color: Color.popups.background
        z: 10

        MouseArea {
          anchors.fill: parent
          enabled: setupWait.visible
          hoverEnabled: true
        }

        Column {
          anchors.centerIn: parent
          spacing: 16
          width: parent.width - 32

          Item {
            width: parent.width
            height: 22

            Item {
              id: spinner
              width: 22
              height: 22
              anchors.horizontalCenter: parent.horizontalCenter

              Canvas {
                id: spinMark
                anchors.fill: parent
                onPaint: {
                  var ctx = getContext("2d")
                  ctx.reset()
                  ctx.strokeStyle = root.ink
                  ctx.lineWidth = 1.5
                  ctx.lineCap = "square"
                  var m = width / 2
                  ctx.beginPath()
                  ctx.arc(m, m, Math.max(1, m - 2), -Math.PI / 2, Math.PI)
                  ctx.stroke()
                }
                onWidthChanged: requestPaint()
                onHeightChanged: requestPaint()
                Component.onCompleted: requestPaint()
              }

              Connections {
                target: root
                function onInkChanged() { spinMark.requestPaint() }
              }

              RotationAnimation on rotation {
                running: setupWait.visible
                from: 0
                to: 360
                duration: 900
                loops: Animation.Infinite
              }
            }
          }

          Text {
            width: parent.width
            text: "Finishing setup"
            color: root.ink
            font.family: root.plex
            font.pixelSize: 13
            horizontalAlignment: Text.AlignHCenter
            textFormat: Text.PlainText
          }
        }
      }
    }
  }

  onSnapshotsChanged: if (faceLoader.item && "snapshots" in faceLoader.item) faceLoader.item.snapshots = snapshots
  onSelectedIndexChanged: if (faceLoader.item && "selectedIndex" in faceLoader.item) faceLoader.item.selectedIndex = selectedIndex
  onHouseholdPinSetChanged: if (faceLoader.item && "householdPinSet" in faceLoader.item) faceLoader.item.householdPinSet = householdPinSet
  onHour12Changed: if (faceLoader.item && "hour12" in faceLoader.item) faceLoader.item.hour12 = hour12
  onStatusJsonChanged: if (faceLoader.item && "statusJson" in faceLoader.item) faceLoader.item.statusJson = statusJson
  onLookPilesChanged: if (faceLoader.item && "lookPiles" in faceLoader.item) faceLoader.item.lookPiles = lookPiles
  onHostWidgetChanged: if (faceLoader.item && "hostWidget" in faceLoader.item) faceLoader.item.hostWidget = hostWidget
  onBarChanged: if (faceLoader.item && "bar" in faceLoader.item) faceLoader.item.bar = bar
}
