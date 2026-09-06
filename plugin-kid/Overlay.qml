import QtQuick
import Quickshell
import Quickshell.Io
import Quickshell.Wayland
import qs.Commons
import "KidModel.js" as Model

Item {
  id: root

  property var shell: null
  property var settings: ({})
  property var statusJson: ({})
  property var bank: ({ url: "http://127.0.0.1:8742", readToken: "", askToken: "" })
  property string face: ""
  property string step: "cover"
  property string pinDigits: ""
  property int chosenMinutes: 30
  property bool stayAwakeOurs: false
  property bool submapOn: false
  property bool pinWrong: false
  property bool askQueued: false

  readonly property bool shown: Model.overlayVisible(statusJson)
  readonly property bool waiting: askQueued || Model.overlayAskWaiting(statusJson)
  readonly property color ink: Color.foreground
  readonly property color paper: Color.background
  readonly property color urgent: Color.urgent
  readonly property color dim: Qt.darker(ink, 1.4)
  readonly property string plex: "IBM Plex Mono"

  function bankUrl() {
    return String(bank.url || "http://127.0.0.1:8742").replace(/\/$/, "")
  }

  function loadShell(raw) {
    try {
      bank = Model.kidSettingsFromShell(JSON.parse(raw))
    } catch (e) {
      bank = Model.kidSettingsFromShell({})
    }
  }

  function poll() {
    var req = new XMLHttpRequest()
    req.open("GET", bankUrl() + "/v1/status")
    req.setRequestHeader("Authorization", "Bearer " + String(bank.readToken || ""))
    req.onreadystatechange = function() {
      if (req.readyState !== XMLHttpRequest.DONE || req.status !== 200) return
      try {
        statusJson = Model.parseStatus(JSON.parse(req.responseText))
      } catch (e) {
        return
      }
      face = Model.overlayFace(statusJson)
      if (!shown) {
        step = "cover"
        pinDigits = ""
        pinWrong = false
        askQueued = false
        chosenMinutes = Model.ASK_DEFAULT_MIN
      } else if (Model.overlayAskWaiting(statusJson)) {
        askQueued = false
        if (step === "ask") step = "cover"
      }
    }
    try {
      req.send()
    } catch (e) {
    }
  }

  function grant() {
    if (!Model.validPin(pinDigits)) return
    var body = Model.pinGrantPayload(pinDigits, chosenMinutes * 60)
    var req = new XMLHttpRequest()
    req.open("POST", bankUrl() + "/v1/pin/grant")
    req.setRequestHeader("Authorization", "Bearer " + String(bank.askToken || ""))
    req.setRequestHeader("Content-Type", "application/json")
    req.onreadystatechange = function() {
      if (req.readyState !== XMLHttpRequest.DONE) return
      if (req.status !== 200) {
        pinWrong = true
        step = "pin"
        return
      }
      pinDigits = ""
      pinWrong = false
      step = "cover"
      poll()
    }
    req.send(JSON.stringify(body))
  }

  function submitAsk() {
    if (root.waiting) return
    askQueued = true
    var body = Model.askPayload("fun", chosenMinutes * 60, "more time")
    var req = new XMLHttpRequest()
    req.open("POST", bankUrl() + "/v1/asks")
    req.setRequestHeader("Authorization", "Bearer " + String(bank.askToken || ""))
    req.setRequestHeader("Content-Type", "application/json")
    req.onreadystatechange = function() {
      if (req.readyState !== XMLHttpRequest.DONE) return
      if (req.status !== 200) askQueued = false
      step = "cover"
      if (req.status === 200) poll()
    }
    req.send(JSON.stringify(body))
  }

  function hypr(batch) {
    hyprProc.command = ["hyprctl", "--batch", batch]
    hyprProc.running = true
  }

  function enterKidProof() {
    if (submapOn) return
    hypr([
      "keyword submap kidtimeroverlay",
      "keyword bind SUPER,catchall,exec,true",
      "keyword bind SUPER SHIFT,catchall,exec,true",
      "keyword bind SUPER CONTROL,catchall,exec,true",
      "keyword bind SUPER ALT,catchall,exec,true",
      "keyword bind SUPER CONTROL SHIFT,catchall,exec,true",
      "keyword bind SUPER ALT SHIFT,catchall,exec,true",
      "keyword bind SUPER CONTROL ALT,catchall,exec,true",
      "keyword bind SUPER CONTROL ALT SHIFT,catchall,exec,true",
      "keyword submap reset",
      "dispatch submap kidtimeroverlay"
    ].join(" ; "))
    submapOn = true
    stayProc.command = ["bash", "-c", 'f="$HOME/.local/state/omarchy/indicators/stay-awake"; if [[ -f "$f" ]]; then echo existed; else mkdir -p "$(dirname "$f")" && touch "$f" && echo created; fi']
    stayProc.running = true
  }

  function leaveKidProof() {
    if (submapOn) {
      hypr("dispatch submap reset")
      submapOn = false
    }
    if (stayAwakeOurs) {
      stayProc.command = ["bash", "-c", 'rm -f "$HOME/.local/state/omarchy/indicators/stay-awake"']
      stayProc.running = true
      stayAwakeOurs = false
    }
  }

  onShownChanged: {
    if (shown) enterKidProof()
    else leaveKidProof()
  }

  Component.onDestruction: leaveKidProof()

  FileView {
    id: shellFile
    path: Quickshell.env("HOME") + "/.config/omarchy/shell.json"
    watchChanges: true
    printErrors: false
    onLoaded: root.loadShell(text())
    onFileChanged: shellFile.reload()
  }

  Timer {
    interval: 1000
    running: true
    repeat: true
    triggeredOnStart: true
    onTriggered: root.poll()
  }

  Process { id: hyprProc }
  Process {
    id: stayProc
    stdout: StdioCollector {
      waitForEnd: true
      onStreamFinished: {
        if (text.indexOf("created") >= 0) root.stayAwakeOurs = true
      }
    }
  }

  PanelWindow {
    id: overlay
    visible: root.shown
    anchors { top: true; bottom: true; left: true; right: true }
    color: root.paper
    WlrLayershell.namespace: "kidtimer-kid-overlay"
    WlrLayershell.layer: WlrLayer.Overlay
    WlrLayershell.keyboardFocus: root.shown ? WlrKeyboardFocus.Exclusive : WlrKeyboardFocus.None
    exclusionMode: ExclusionMode.Ignore

    Keys.onPressed: function(event) {
      if (event.key === Qt.Key_Escape) event.accepted = true
    }

    Rectangle {
      anchors.fill: parent
      color: root.paper

      Column {
        anchors.centerIn: parent
        width: Math.min(420, parent.width - 48)
        spacing: 18

        Text {
          visible: root.step === "cover" && root.face === "locked"
          width: parent.width
          horizontalAlignment: Text.AlignHCenter
          text: "locked"
          color: root.urgent
          font.family: root.plex
          font.pixelSize: 28
          font.bold: true
        }

        Text {
          visible: root.step === "cover" && root.face === "bedtime"
          width: parent.width
          horizontalAlignment: Text.AlignHCenter
          text: "bedtime"
          color: "#c5c9ef"
          font.family: root.plex
          font.pixelSize: 28
          font.bold: true
        }

        Item {
          visible: root.step === "cover" && root.face === "bedtime"
          width: parent.width
          height: 48
          clip: true
          Rectangle { anchors.fill: parent; color: "#1c1e30" }
          Repeater {
            model: 48
            Rectangle {
              required property int index
              width: 6
              height: 96
              rotation: -52
              x: index * 10 - 48
              y: -24
              color: index % 2 ? Qt.rgba(70 / 255, 74 / 255, 140 / 255, 0.62) : Qt.rgba(20 / 255, 22 / 255, 34 / 255, 0.55)
            }
          }
          Text {
            anchors.centerIn: parent
            text: "lights out"
            color: Qt.rgba(root.ink.r, root.ink.g, root.ink.b, 0.7)
            font.family: root.plex
            font.pixelSize: 11
            font.capitalization: Font.AllUppercase
          }
        }

        Text {
          visible: root.step === "cover" && root.face === "bedtime"
          width: parent.width
          horizontalAlignment: Text.AlignHCenter
          text: "until " + Model.bedtimeEnd(root.statusJson)
          color: root.ink
          font.family: root.plex
          font.pixelSize: 16
        }

        Text {
          visible: root.step === "cover" && root.face === "empty"
          width: parent.width
          horizontalAlignment: Text.AlignHCenter
          text: "0m LEFT"
          color: root.urgent
          font.family: root.plex
          font.pixelSize: 28
          font.bold: true
        }

        Item {
          visible: root.step === "cover"
          width: parent.width
          height: 36
          Rectangle {
            width: (parent.width - 8) / 2
            height: parent.height
            anchors.left: parent.left
            color: "transparent"
            border.width: 1
            border.color: root.waiting ? root.dim : root.ink
            opacity: root.waiting ? 0.55 : 1
            Text {
              anchors.centerIn: parent
              text: root.waiting ? "Waiting" : "Ask"
              color: root.waiting ? root.dim : root.ink
              font.family: root.plex
              font.pixelSize: 14
              font.bold: true
            }
            MouseArea {
              anchors.fill: parent
              enabled: !root.waiting
              cursorShape: enabled ? Qt.PointingHandCursor : Qt.ArrowCursor
              onClicked: {
                root.chosenMinutes = Model.ASK_DEFAULT_MIN
                root.step = "ask"
              }
            }
          }
          Rectangle {
            width: (parent.width - 8) / 2
            height: parent.height
            anchors.right: parent.right
            color: "transparent"
            border.width: 1
            border.color: root.ink
            Text {
              anchors.centerIn: parent
              text: "Parent Pin"
              color: root.ink
              font.family: root.plex
              font.pixelSize: 14
              font.bold: true
            }
            MouseArea {
              anchors.fill: parent
              cursorShape: Qt.PointingHandCursor
              onClicked: {
                root.step = "pin"
                root.pinWrong = false
                pinField.forceActiveFocus()
              }
            }
          }
        }

        Text {
          visible: root.step === "pin"
          width: parent.width
          horizontalAlignment: Text.AlignHCenter
          text: root.pinWrong ? "wrong pin" : "Parent Pin"
          color: root.pinWrong ? root.urgent : root.ink
          font.family: root.plex
          font.pixelSize: 18
          font.bold: true
        }

        TextInput {
          id: pinField
          visible: root.step === "pin"
          width: parent.width
          height: 40
          horizontalAlignment: Text.AlignHCenter
          color: root.ink
          font.family: root.plex
          font.pixelSize: 28
          echoMode: TextInput.Password
          inputMethodHints: Qt.ImhDigitsOnly
          maximumLength: 4
          text: root.pinDigits
          onTextChanged: root.pinDigits = text
          Keys.onReturnPressed: {
            if (Model.validPin(root.pinDigits)) {
              root.chosenMinutes = Model.ASK_DEFAULT_MIN
              root.step = "minutes"
            }
          }
        }

        Item {
          visible: root.step === "minutes"
          width: parent.width
          height: 48
          Rectangle {
            width: 56
            height: 40
            anchors.left: parent.left
            color: "transparent"
            border.width: 1
            border.color: root.ink
            Text { anchors.centerIn: parent; text: "−5"; color: root.ink; font.family: root.plex; font.pixelSize: 16 }
            MouseArea {
              anchors.fill: parent
              onClicked: root.chosenMinutes = Model.nudgeAskMinutes(root.chosenMinutes, -5)
            }
          }
          Column {
            anchors.centerIn: parent
            Text {
              anchors.horizontalCenter: parent.horizontalCenter
              text: Model.overlayStepperLabel(root.chosenMinutes)
              color: root.ink
              font.family: root.plex
              font.pixelSize: 28
              font.bold: true
            }
            Text {
              anchors.horizontalCenter: parent.horizontalCenter
              text: "min"
              color: root.dim
              font.family: root.plex
              font.pixelSize: 11
            }
          }
          Rectangle {
            width: 56
            height: 40
            anchors.right: parent.right
            color: "transparent"
            border.width: 1
            border.color: root.ink
            Text { anchors.centerIn: parent; text: "+5"; color: root.ink; font.family: root.plex; font.pixelSize: 16 }
            MouseArea {
              anchors.fill: parent
              onClicked: root.chosenMinutes = Model.nudgeAskMinutes(root.chosenMinutes, 5)
            }
          }
        }

        Rectangle {
          visible: root.step === "minutes"
          width: parent.width
          height: 36
          color: "transparent"
          border.width: 1
          border.color: root.ink
          Text {
            anchors.centerIn: parent
            text: "Give time"
            color: root.ink
            font.family: root.plex
            font.pixelSize: 14
            font.bold: true
          }
          MouseArea {
            anchors.fill: parent
            onClicked: root.grant()
          }
        }

        Item {
          visible: root.step === "ask"
          width: parent.width
          height: 48
          Rectangle {
            width: 56
            height: 40
            anchors.left: parent.left
            color: "transparent"
            border.width: 1
            border.color: root.ink
            opacity: root.chosenMinutes <= Model.OVERLAY_ASK_MIN ? 0.55 : 1
            Text { anchors.centerIn: parent; text: "−10"; color: root.ink; font.family: root.plex; font.pixelSize: 16 }
            MouseArea {
              anchors.fill: parent
              onClicked: root.chosenMinutes = Model.nudgeOverlayAskMinutes(root.chosenMinutes, -10)
            }
          }
          Column {
            anchors.centerIn: parent
            Text {
              anchors.horizontalCenter: parent.horizontalCenter
              text: Model.overlayAskStepperLabel(root.chosenMinutes)
              color: root.ink
              font.family: root.plex
              font.pixelSize: 28
              font.bold: true
            }
            Text {
              anchors.horizontalCenter: parent.horizontalCenter
              text: "min"
              color: root.dim
              font.family: root.plex
              font.pixelSize: 11
            }
          }
          Rectangle {
            width: 56
            height: 40
            anchors.right: parent.right
            color: "transparent"
            border.width: 1
            border.color: root.ink
            opacity: root.chosenMinutes >= Model.ASK_MAX ? 0.55 : 1
            Text { anchors.centerIn: parent; text: "+10"; color: root.ink; font.family: root.plex; font.pixelSize: 16 }
            MouseArea {
              anchors.fill: parent
              onClicked: root.chosenMinutes = Model.nudgeOverlayAskMinutes(root.chosenMinutes, 10)
            }
          }
        }

        Item {
          visible: root.step === "ask"
          width: parent.width
          height: 36
          Rectangle {
            width: (parent.width - 8) / 2
            height: parent.height
            anchors.left: parent.left
            color: "transparent"
            border.width: 1
            border.color: root.ink
            Text {
              anchors.centerIn: parent
              text: "Cancel"
              color: root.ink
              font.family: root.plex
              font.pixelSize: 14
              font.bold: true
            }
            MouseArea {
              anchors.fill: parent
              cursorShape: Qt.PointingHandCursor
              onClicked: {
                root.step = "cover"
                root.chosenMinutes = Model.ASK_DEFAULT_MIN
              }
            }
          }
          Rectangle {
            width: (parent.width - 8) / 2
            height: parent.height
            anchors.right: parent.right
            color: "transparent"
            border.width: 1
            border.color: root.ink
            Text {
              anchors.centerIn: parent
              text: "Ask"
              color: root.ink
              font.family: root.plex
              font.pixelSize: 14
              font.bold: true
            }
            MouseArea {
              anchors.fill: parent
              cursorShape: Qt.PointingHandCursor
              onClicked: root.submitAsk()
            }
          }
        }
      }
    }
  }
}
