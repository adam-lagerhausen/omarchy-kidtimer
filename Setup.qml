import QtQuick
import qs.Commons
import qs.Ui

Item {
  id: root
  property var bar: null
  property var hostWidget: null
  property string setupError: hostWidget ? String(hostWidget.setupError || "") : ""
  readonly property bool busy: hostWidget ? hostWidget.setupBusy === true : false

  readonly property color ink: bar ? bar.foreground : Color.popups.text
  readonly property color surface: Color.popups.background
  readonly property color quiet: {
    if (colorLuminance(surface) < 0.5)
      return Qt.darker(ink, 1.4)
    return Qt.rgba(ink.r * 0.45 + surface.r * 0.55, ink.g * 0.45 + surface.g * 0.55, ink.b * 0.45 + surface.b * 0.55, 1)
  }
  FontLoader { id: plexReg; source: Qt.resolvedUrl("fonts/JetBrainsMono-Regular.ttf") }
  FontLoader { id: plexMed; source: Qt.resolvedUrl("fonts/JetBrainsMono-Medium.ttf") }
  FontLoader { id: plexSemi; source: Qt.resolvedUrl("fonts/JetBrainsMono-SemiBold.ttf") }
  readonly property string plex: {
    if (hostWidget && hostWidget.plex) return hostWidget.plex
    return plexReg.status === FontLoader.Ready ? plexReg.name : "JetBrains Mono"
  }

  function colorLuminance(c) {
    var r = c.r, g = c.g, b = c.b
    return 0.2126 * r + 0.7152 * g + 0.0722 * b
  }

  function roleHost() {
    if (root.hostWidget) return root.hostWidget
    var p = parent
    for (var n = 0; n < 12 && p; n++) {
      if (p.hostWidget) return p.hostWidget
      p = p.parent
    }
    return null
  }

  width: parent ? parent.width : 340
  implicitHeight: col.implicitHeight + 28

  Column {
    id: col
    x: 16
    y: 14
    width: parent.width - 32
    spacing: 14

    Text {
      width: parent.width
      text: "Kidtimer"
      color: ink
      font.family: root.plex
      font.pixelSize: 18
      font.weight: Font.DemiBold
      textFormat: Text.PlainText
    }

    Text {
      width: parent.width
      text: "Which computer is this?"
      color: quiet
      font.family: root.plex
      font.pixelSize: 13
      wrapMode: Text.WordWrap
      textFormat: Text.PlainText
    }

    LookBtn {
      width: parent.width
      height: 44
      text: "This is mine"
      primary: true
      fontFamily: root.plex
      fontSize: 14
      enabled: !root.busy
      onClicked: { var host = roleHost(); if (host) host.pickRole("parent") }
    }

    LookBtn {
      width: parent.width
      height: 44
      text: "This is the kid's"
      fontFamily: root.plex
      fontSize: 14
      enabled: !root.busy
      onClicked: { var host = roleHost(); if (host) host.pickRole("kid") }
    }

    Text {
      visible: root.setupError !== ""
      width: parent.width
      text: root.setupError
      color: Color.urgent
      font.family: root.plex
      font.pixelSize: 12
      wrapMode: Text.WordWrap
      textFormat: Text.PlainText
    }
  }
}
