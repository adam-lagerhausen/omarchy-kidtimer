import QtQuick
import qs.Commons

Rectangle {
  id: root

  property string text: ""
  property bool selected: false
  property bool ghost: false
  property bool dashed: false
  property bool held: false
  property bool danger: false
  property bool primary: false
  property bool leftAlign: false
  property color foreground: Color.foreground
  property color accent: Color.accent
  property color urgent: Color.urgent
  property string fontFamily: Style.font.family
  property real fontSize: 12
  property bool bold: false
  property string kind: "plain"
  property string icon: ""
  property color swatch: "transparent"
  property bool hasSwatch: false

  signal clicked()

  readonly property color fill: Qt.rgba(foreground.r, foreground.g, foreground.b, 0.18)
  readonly property color fillSoft: Qt.rgba(foreground.r, foreground.g, foreground.b, 0.04)
  readonly property color line: Qt.rgba(foreground.r, foreground.g, foreground.b, 0.4)
  readonly property color hover: Qt.rgba(foreground.r, foreground.g, foreground.b, 0.08)
  readonly property bool hot: mouse.containsMouse && root.enabled
  readonly property int box: {
    if (kind === "nudge") return 28
    if (kind === "nudge-sm" || kind === "plus") return 22
    if (kind === "dow") return 26
    if (kind === "x") return 18
    if (kind === "lock") return 36
    return 0
  }
  readonly property int padX: {
    if (kind === "page") return 8
    if (box > 0) return 0
    return 10
  }
  readonly property int padY: {
    if (kind === "page") return 5
    if (box > 0) return 0
    return 6
  }
  readonly property real useFont: {
    if (kind === "page" || kind === "dow") return 11
    if (kind === "nudge") return 16
    if (kind === "nudge-sm") return 13
    return fontSize
  }

  radius: 0
  opacity: enabled ? 1 : 0.35
  color: {
    if (primary) return hot ? Qt.lighter(accent, 1.12) : accent
    if (held) return Qt.rgba(urgent.r, urgent.g, urgent.b, 0.22)
    if (selected) return fill
    if (ghost || dashed) return hot ? hover : "transparent"
    if (hot) return hover
    return fillSoft
  }
  border.width: (selected || held || dashed || primary) ? 0 : 1
  border.color: {
    if (danger) return Qt.rgba(urgent.r, urgent.g, urgent.b, 0.45)
    if (kind === "plus") return Qt.rgba(accent.r, accent.g, accent.b, 0.55)
    return line
  }

  implicitWidth: box > 0 ? box : (row.implicitWidth + padX * 2)
  implicitHeight: box > 0 ? box : (Math.max(row.implicitHeight + padY * 2, 28))

  Canvas {
    id: dash
    anchors.fill: parent
    visible: root.dashed
    onPaint: {
      var ctx = getContext("2d")
      ctx.reset()
      ctx.strokeStyle = root.kind === "plus"
        ? Qt.rgba(root.accent.r, root.accent.g, root.accent.b, 0.55)
        : root.line
      ctx.lineWidth = 1
      ctx.setLineDash([3, 3])
      ctx.strokeRect(0.5, 0.5, width - 1, height - 1)
    }
    onWidthChanged: requestPaint()
    onHeightChanged: requestPaint()
  }

  Row {
    id: row
    anchors.centerIn: leftAlign ? undefined : parent
    anchors.left: leftAlign ? parent.left : undefined
    anchors.leftMargin: leftAlign ? padX : 0
    anchors.verticalCenter: parent.verticalCenter
    spacing: 10

    Rectangle {
      visible: root.hasSwatch
      width: 7
      height: 7
      radius: 4
      color: root.swatch
      anchors.verticalCenter: parent.verticalCenter
    }

    Item {
      visible: root.kind === "lock" || root.icon !== ""
      width: 14
      height: 16
      Canvas {
        id: lockIco
        anchors.fill: parent
        visible: root.kind === "lock"
        onPaint: {
          var ctx = getContext("2d")
          ctx.reset()
          ctx.strokeStyle = root.held ? root.urgent : root.foreground
          ctx.fillStyle = root.held ? root.urgent : root.foreground
          ctx.lineWidth = 1.7
          ctx.lineCap = "round"
          ctx.beginPath()
          if (root.held) {
            ctx.moveTo(4.4, 7)
            ctx.lineTo(4.4, 4.2)
            ctx.arc(7.2, 4.2, 2.8, Math.PI, 0, false)
            ctx.lineTo(10, 7)
          } else {
            ctx.moveTo(5, 7)
            ctx.lineTo(5, 4.2)
            ctx.arc(8.6, 4.2, 2.8, Math.PI, 0.35, false)
          }
          ctx.stroke()
          ctx.fillRect(2, 7, 10, 8)
        }
        Connections {
          target: root
          function onHeldChanged() { lockIco.requestPaint() }
          function onForegroundChanged() { lockIco.requestPaint() }
        }
      }
      Text {
        visible: root.icon !== "" && root.kind !== "lock"
        anchors.centerIn: parent
        text: root.icon
        color: root.foreground
        font.family: root.fontFamily
        font.pixelSize: 14
      }
    }

    Text {
      id: label
      visible: root.text !== ""
      text: root.text
      color: root.primary ? "#13151b" : (root.held ? root.urgent : (root.kind === "plus" ? root.accent : root.foreground))
      font.family: root.fontFamily
      font.pixelSize: root.useFont
      font.bold: root.bold || root.kind === "lock"
    }
  }

  MouseArea {
    id: mouse
    anchors.fill: parent
    enabled: root.enabled
    hoverEnabled: true
    cursorShape: enabled ? Qt.PointingHandCursor : Qt.ArrowCursor
    onClicked: root.clicked()
  }
}
