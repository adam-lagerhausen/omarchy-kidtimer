import QtQuick
import Quickshell
import Quickshell.Wayland
import qs.Commons

Item {
  id: root
  property var hostWidget: null
  readonly property var pings: hostWidget && hostWidget.pingQueue ? hostWidget.pingQueue : []
  readonly property bool shown: !!(hostWidget && hostWidget.role === "parent" && pings.length > 0)
  readonly property string plex: hostWidget && hostWidget.plex ? hostWidget.plex : "JetBrains Mono"
  readonly property color ink: Color.foreground
  readonly property color paper: Color.background
  readonly property color quiet: Qt.darker(ink, 1.4)
  readonly property color hover: Qt.rgba(ink.r, ink.g, ink.b, 0.08)

  function decide(card, decision) {
    if (!hostWidget || !hostWidget.decidePing) return
    hostWidget.decidePing(card, decision)
  }

  PanelWindow {
    visible: root.shown
    color: "transparent"
    anchors { top: true; bottom: true; left: true; right: true }
    WlrLayershell.namespace: "kidtimer-parent-ping"
    WlrLayershell.layer: WlrLayer.Overlay
    WlrLayershell.keyboardFocus: WlrKeyboardFocus.None
    exclusionMode: ExclusionMode.Ignore
    mask: Region { item: pingCol }

    Column {
      id: pingCol
      anchors.right: parent.right
      anchors.top: parent.top
      anchors.topMargin: 36
      anchors.rightMargin: 12
      width: 320
      spacing: 8

      Repeater {
        model: root.pings
        Rectangle {
          id: pingCard
          required property var modelData
          width: 320
          implicitHeight: pingInner.implicitHeight + 24
          height: implicitHeight
          color: root.paper
          border.width: 1
          border.color: root.ink
          radius: 0

          Column {
            id: pingInner
            x: 12
            y: 12
            width: parent.width - 24
            spacing: 0

            Text {
              textFormat: Text.PlainText
              text: "KIDTIMER"
              color: root.quiet
              font.family: root.plex
              font.pixelSize: 10
              font.letterSpacing: 0.4
            }
            Text {
              textFormat: Text.PlainText
              topPadding: 6
              text: String(pingCard.modelData.head || "")
              color: root.ink
              font.family: root.plex
              font.pixelSize: 13
              font.weight: Font.DemiBold
            }
            Text {
              textFormat: Text.PlainText
              topPadding: 4
              bottomPadding: 10
              width: parent.width
              wrapMode: Text.WordWrap
              text: String(pingCard.modelData.body || "")
              color: root.ink
              font.family: root.plex
              font.pixelSize: 12
            }
            Row {
              width: parent.width
              spacing: 8
              Repeater {
                model: [
                  { decision: "deny", label: "DENY" },
                  { decision: "approve", label: "APPROVE" }
                ]
                Rectangle {
                  required property var modelData
                  readonly property string decision: String(modelData.decision || "")
                  width: (pingInner.width - 8) / 2
                  height: 28
                  color: actMouse.containsMouse ? root.hover : "transparent"
                  border.width: 1
                  border.color: root.ink
                  radius: 0
                  Text {
                    textFormat: Text.PlainText
                    anchors.centerIn: parent
                    text: String(modelData.label || "")
                    color: root.ink
                    font.family: root.plex
                    font.pixelSize: 11
                    font.letterSpacing: 0.4
                  }
                  MouseArea {
                    id: actMouse
                    anchors.fill: parent
                    hoverEnabled: true
                    cursorShape: Qt.PointingHandCursor
                    onClicked: root.decide(pingCard.modelData, decision)
                  }
                }
              }
            }
          }
        }
      }
    }
  }
}
