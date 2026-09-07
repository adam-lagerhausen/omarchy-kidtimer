import QtQuick
import qs.Commons
import qs.Ui
import "KidModel.js" as Model

Item {
  id: root

  property var bar: null
  property var anchorItem: null
  property var hostWidget: null
  property var statusJson: ({})
  property var lookPiles: []
  property string askGroup: ""
  property int chosenMinutes: 30
  property string waitingGroup: ""
  property string pendingAskId: ""
  property bool pinOpen: false
  property string pinDigits: ""
  property bool pinWrong: false
  property int lastPending: -1

  readonly property var barIdentity: hostWidget || root
  readonly property color contentForeground: bar ? bar.foreground : Color.foreground
  FontLoader { id: plexReg; source: Qt.resolvedUrl("fonts/JetBrainsMono-Regular.ttf") }
  FontLoader { id: plexMed; source: Qt.resolvedUrl("fonts/JetBrainsMono-Medium.ttf") }
  FontLoader { id: plexSemi; source: Qt.resolvedUrl("fonts/JetBrainsMono-SemiBold.ttf") }
  readonly property string contentFontFamily: plexReg.status === FontLoader.Ready ? plexReg.name : "JetBrains Mono"
  readonly property color accent: "#1daeeb"
  readonly property color urgent: Color.urgent
  readonly property color dim: Qt.darker(contentForeground, 1.4)
  readonly property color fillSoft: Qt.rgba(contentForeground.r, contentForeground.g, contentForeground.b, 0.04)
  readonly property color line: Qt.rgba(contentForeground.r, contentForeground.g, contentForeground.b, 0.4)
  readonly property bool blocked: Model.askBlocked(statusJson)
  readonly property string view: {
    if (root.pinOpen) return "pin"
    if (root.askGroup !== "") return "sheet"
    return Model.panelKind(root.statusJson)
  }
  readonly property var clock: Model.clockFace(root.statusJson, root.waitingGroup !== "")
  readonly property string soonBanner: Model.bedtimeBanner(root.statusJson)
  readonly property color panelLine: {
    if (root.view === "locked") return root.urgent
    if (root.view === "bedtime") return "#7a82c4"
    return root.accent
  }
  width: parent ? parent.width : 340
  implicitHeight: Math.max(1, bodyHeight) + 14 + 18

  function setting(key, fallback) {
    if (hostWidget && typeof hostWidget.setting === "function")
      return hostWidget.setting(key, fallback)
    var bank = hostWidget && hostWidget.kidBank
    if (bank && key in bank) return bank[key]
    return fallback
  }

  readonly property real bodyHeight: {
    if (root.view === "sheet") return sheetColumn.implicitHeight
    if (root.view === "pin") return pinColumn.implicitHeight
    if (root.view === "locked") return lockColumn.implicitHeight
    if (root.view === "bedtime") return bedColumn.implicitHeight
    return homeColumn.implicitHeight
  }



  onStatusJsonChanged: {
    var pending = Number(statusJson && statusJson.pending_ask_count) || 0
    if (root.waitingGroup !== "" && pending === 0 && root.lastPending > 0) {
      root.waitingGroup = ""
      root.pendingAskId = ""
      root.pinOpen = false
      root.pinDigits = ""
    }
    root.lastPending = pending
    if (Model.askBlocked(statusJson)) root.askGroup = ""
  }

  function bankUrl() {
    return String(setting("url", "http://127.0.0.1:8742")).replace(/\/$/, "")
  }

  function openAsk() {
    if (Model.askBlocked(root.statusJson)) return
    root.askGroup = "fun"
    root.chosenMinutes = Model.ASK_DEFAULT_MIN
  }

  function submit() {
    if (root.askGroup === "") return
    if (Model.askBlocked(root.statusJson)) return
    var body = Model.askPayload(root.askGroup, root.chosenMinutes * 60, "more time")
    if (!hostWidget || typeof hostWidget.kidPost !== "function") return
    hostWidget.kidPost("/v1/asks", body, String(setting("askToken", "")), function(status, text) {
      if (status !== 200) {
        root.askGroup = ""
        return
      }
      var id = ""
      try {
        id = JSON.parse(text).id || ""
      } catch (e) {
        id = ""
      }
      root.pendingAskId = id
      root.waitingGroup = root.askGroup
      root.askGroup = ""
    })
  }

  function submitPin() {
    if (!Model.validPin(root.pinDigits) || root.pendingAskId === "") return
    var body = Model.pinApprovePayload(root.pinDigits, root.pendingAskId)
    if (!hostWidget || typeof hostWidget.kidPost !== "function") return
    hostWidget.kidPost("/v1/pin/approve", body, String(setting("askToken", "")), function(status, text) {
      if (status !== 200) {
        root.pinWrong = true
        return
      }
      root.pinOpen = false
      root.pinDigits = ""
      root.pinWrong = false
      root.waitingGroup = ""
      root.pendingAskId = ""
    })
  }

  Item {
    id: panel
    anchors.fill: parent
    anchors.leftMargin: 18
    anchors.rightMargin: 18
    anchors.topMargin: 14
    anchors.bottomMargin: 18

      Column {
        id: homeColumn
        visible: root.view === "home"
        width: parent.width
        spacing: Style.space(12)

        Rectangle {
          visible: root.soonBanner !== ""
          width: parent.width
          height: bannerText.implicitHeight + 16
          color: Qt.rgba(root.urgent.r, root.urgent.g, root.urgent.b, 0.1)
          border.width: 1
          border.color: Qt.rgba(root.urgent.r, root.urgent.g, root.urgent.b, 0.55)
          Text {
            textFormat: Text.PlainText
            id: bannerText
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.verticalCenter: parent.verticalCenter
            anchors.leftMargin: 10
            anchors.rightMargin: 10
            text: root.soonBanner
            color: root.urgent
            font.family: root.contentFontFamily
            font.pixelSize: 11
          }
        }

        Text {
          textFormat: Text.PlainText
          width: parent.width
          text: root.clock.leftLabel
          color: root.clock.empty ? root.urgent : root.contentForeground
          font.family: root.contentFontFamily
          font.pixelSize: 22
          font.bold: true
          font.letterSpacing: -0.6
        }

        Rectangle {
          width: parent.width
          height: 10
          color: root.fillSoft
          border.width: 1
          border.color: root.clock.empty ? Qt.rgba(root.urgent.r, root.urgent.g, root.urgent.b, 0.7) : root.line
          Item {
            anchors.fill: parent
            anchors.margins: 1
            Rectangle {
              width: Math.max(0, parent.width * root.clock.fill)
              height: parent.height
              color: root.clock.empty ? root.urgent : root.contentForeground
            }
          }
        }

        LookBtn {
          visible: !root.clock.waiting
          width: parent.width
          text: "Ask"
          enabled: !root.blocked
          foreground: root.contentForeground
          fontFamily: root.contentFontFamily
          onClicked: root.openAsk()
        }

        Item {
          visible: root.clock.waiting
          width: parent.width
          height: 28
          LookBtn {
            anchors.left: parent.left
            width: (parent.width - 8) / 2
            text: "Waiting"
            enabled: false
            foreground: root.contentForeground
            fontFamily: root.contentFontFamily
          }
          LookBtn {
            anchors.right: parent.right
            width: (parent.width - 8) / 2
            text: "Parent Pin"
            enabled: root.pendingAskId !== ""
            foreground: root.contentForeground
            fontFamily: root.contentFontFamily
            onClicked: {
              root.pinOpen = true
              root.pinWrong = false
              root.pinDigits = ""
            }
          }
        }
      }

      Column {
        id: sheetColumn
        visible: root.view === "sheet"
        width: parent.width
        spacing: Style.space(12)

        Text {
          textFormat: Text.PlainText
          text: "Ask for more"
          color: root.dim
          font.family: root.contentFontFamily
          font.pixelSize: Style.font.caption
          font.letterSpacing: 1
          font.bold: true
          font.capitalization: Font.AllUppercase
        }

        Item {
          width: parent.width
          height: 40

          LookBtn {
            width: 56
            height: 40
            anchors.left: parent.left
            text: "−5"
            enabled: root.chosenMinutes > 5
            foreground: root.contentForeground
            fontFamily: root.contentFontFamily
            onClicked: root.chosenMinutes = Model.nudgeAskMinutes(root.chosenMinutes, -5)
          }

          Column {
            anchors.centerIn: parent
            Text {
              textFormat: Text.PlainText
              anchors.horizontalCenter: parent.horizontalCenter
              text: String(root.chosenMinutes)
              color: root.contentForeground
              font.family: root.contentFontFamily
              font.pixelSize: 28
              font.bold: true
            }
            Text {
              textFormat: Text.PlainText
              anchors.horizontalCenter: parent.horizontalCenter
              text: "min"
              color: root.dim
              font.family: root.contentFontFamily
              font.pixelSize: 11
            }
          }

          LookBtn {
            width: 56
            height: 40
            anchors.right: parent.right
            text: "+5"
            enabled: root.chosenMinutes < 120
            foreground: root.contentForeground
            fontFamily: root.contentFontFamily
            onClicked: root.chosenMinutes = Model.nudgeAskMinutes(root.chosenMinutes, 5)
          }
        }

        Item {
          width: parent.width
          height: 28

          LookBtn {
            anchors.left: parent.left
            text: "Cancel"
            foreground: root.contentForeground
            fontFamily: root.contentFontFamily
            onClicked: root.askGroup = ""
          }

          LookBtn {
            anchors.right: parent.right
            text: "Ask"
            primary: true
            enabled: !root.blocked
            accent: root.accent
            foreground: root.contentForeground
            fontFamily: root.contentFontFamily
            bold: true
            onClicked: root.submit()
          }
        }
      }

      Column {
        id: pinColumn
        visible: root.view === "pin"
        width: parent.width
        spacing: Style.space(12)

        Text {
          textFormat: Text.PlainText
          text: root.pinWrong ? "wrong pin" : "Parent Pin"
          color: root.pinWrong ? root.urgent : root.dim
          font.family: root.contentFontFamily
          font.pixelSize: Style.font.caption
          font.letterSpacing: 1
          font.bold: true
          font.capitalization: Font.AllUppercase
        }

        TextInput {
          id: panelPin
          width: parent.width
          height: 36
          color: root.contentForeground
          font.family: root.contentFontFamily
          font.pixelSize: 22
          echoMode: TextInput.Password
          inputMethodHints: Qt.ImhDigitsOnly
          maximumLength: 4
          text: root.pinDigits
          onTextChanged: root.pinDigits = text
          Keys.onReturnPressed: root.submitPin()
        }

        Item {
          width: parent.width
          height: 28
          LookBtn {
            anchors.left: parent.left
            text: "Cancel"
            foreground: root.contentForeground
            fontFamily: root.contentFontFamily
            onClicked: {
              root.pinOpen = false
              root.pinDigits = ""
              root.pinWrong = false
            }
          }
          LookBtn {
            anchors.right: parent.right
            text: "OK"
            primary: true
            enabled: Model.validPin(root.pinDigits)
            accent: root.accent
            foreground: root.contentForeground
            fontFamily: root.contentFontFamily
            bold: true
            onClicked: root.submitPin()
          }
        }
      }

      Column {
        id: lockColumn
        visible: root.view === "locked"
        width: parent.width
        spacing: 14

        Canvas {
          id: lockIco
          width: 36
          height: 36
          anchors.horizontalCenter: parent.horizontalCenter
          onPaint: {
            var ctx = getContext("2d")
            ctx.reset()
            ctx.scale(36 / 16, 36 / 16)
            ctx.strokeStyle = root.urgent
            ctx.fillStyle = root.urgent
            ctx.lineWidth = 1.7
            ctx.lineCap = "round"
            ctx.beginPath()
            ctx.moveTo(4.4, 7)
            ctx.lineTo(4.4, 4.2)
            ctx.arc(7.2, 4.2, 2.8, Math.PI, 0, false)
            ctx.lineTo(10, 7)
            ctx.stroke()
            ctx.fillRect(2, 7, 10, 8)
          }
          onVisibleChanged: if (visible) requestPaint()
        }

        Text {
          textFormat: Text.PlainText
          anchors.horizontalCenter: parent.horizontalCenter
          text: "locked"
          color: root.urgent
          font.family: root.contentFontFamily
          font.pixelSize: 22
          font.bold: true
        }
      }

      Column {
        id: bedColumn
        visible: root.view === "bedtime"
        width: parent.width
        spacing: Style.space(12)

        Text {
          textFormat: Text.PlainText
          text: "bedtime"
          color: "#c5c9ef"
          font.family: root.contentFontFamily
          font.pixelSize: 22
          font.bold: true
          font.letterSpacing: -0.6
        }

        Item {
          id: hatch
          width: parent.width
          height: 42
          clip: true

          Rectangle {
            anchors.fill: parent
            color: "#1c1e30"
          }

          Repeater {
            model: 40
            Rectangle {
              required property int index
              width: 6
              height: hatch.height * 2
              rotation: -52
              x: index * 10 - hatch.height
              y: -hatch.height / 2
              color: index % 2
                ? Qt.rgba(70 / 255, 74 / 255, 140 / 255, 0.62)
                : Qt.rgba(20 / 255, 22 / 255, 34 / 255, 0.55)
            }
          }

          Text {
            textFormat: Text.PlainText
            anchors.centerIn: parent
            text: "lights out"
            color: Qt.rgba(root.contentForeground.r, root.contentForeground.g, root.contentForeground.b, 0.7)
            font.family: root.contentFontFamily
            font.pixelSize: 10
            font.letterSpacing: 0.8
            font.capitalization: Font.AllUppercase
          }
        }

        Text {
          textFormat: Text.PlainText
          text: "until " + Model.bedtimeEnd(root.statusJson)
          color: root.contentForeground
          font.family: root.contentFontFamily
          font.pixelSize: 16
        }
      }
    }
  }

