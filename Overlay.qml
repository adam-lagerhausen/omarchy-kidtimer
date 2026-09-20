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
  property string role: ""
  property var bank: ({ url: "http://127.0.0.1:8742", readToken: "", askToken: "" })
  property string face: ""
  property string step: "cover"
  property string pinDigits: ""
  property int chosenMinutes: 30
  property bool submapOn: false
  property bool pinWrong: false
  property string pinFailText: "wrong pin"
  property bool pinBusy: false
  property bool askWrong: false
  property string askFailText: "try again"
  property bool askQueued: false
  property int lastPending: -1
  property bool saveDismissed: false
  property var httpQueue: []
  property var httpJob: null
  property string httpBuf: ""

  readonly property bool blocksSuper: Model.overlayBlocksSuper(statusJson)
  readonly property bool shown: root.role === "kid" && Model.overlayWindowOn(statusJson, saveDismissed)
  readonly property bool waiting: askQueued || Model.overlayAskWaiting(statusJson)
  readonly property color ink: Color.foreground
  readonly property color paper: Color.background
  readonly property color urgent: Color.urgent
  readonly property color dim: Qt.darker(ink, 1.4)
  FontLoader { id: plexReg; source: Qt.resolvedUrl("fonts/JetBrainsMono-Regular.ttf") }
  FontLoader { id: plexMed; source: Qt.resolvedUrl("fonts/JetBrainsMono-Medium.ttf") }
  FontLoader { id: plexSemi; source: Qt.resolvedUrl("fonts/JetBrainsMono-SemiBold.ttf") }
  readonly property string plex: plexReg.status === FontLoader.Ready ? plexReg.name : "JetBrains Mono"

  function bankUrl() {
    return String(bank.url || "http://127.0.0.1:8742").replace(/\/$/, "")
  }

  function loadBank(raw) {
    try {
      bank = Model.parseKidBank(JSON.parse(raw))
    } catch (e) {
      bank = Model.parseKidBank({})
    }
  }

  function applyRole(raw) {
    var next = String(raw || "").replace(/\s+/g, "")
    if (next !== "kid" && next !== "parent") next = ""
    if (root.role === next) return
    if (root.role === "kid" && next !== "kid") leaveKidProof()
    root.role = next
  }

  function applyPinDigits(s) {
    var next = String(s || "")
    root.pinDigits = next
    if (pinField && pinField.text !== next) pinField.text = next
  }

  function poll() {
    if (root.role !== "kid") return
    overlayHTTP("GET", bankUrl() + "/v1/status", String(bank.readToken || ""), "", function(status, text) {
      if (status !== 200) return
      try {
        statusJson = Model.parseStatus(JSON.parse(text))
      } catch (e) {
        return
      }
      face = Model.overlayFace(statusJson)
      if (Model.overlayResetDismiss(statusJson)) saveDismissed = false
      var pending = Number(statusJson.pending_ask_count) || 0
      if (!Model.overlayVisible(statusJson)) {
        step = "cover"
        applyPinDigits("")
        pinWrong = false
        pinFailText = "wrong pin"
        pinBusy = false
        askQueued = false
        lastPending = -1
        askWrong = false
        askFailText = "try again"
        chosenMinutes = Model.ASK_DEFAULT_MIN
      } else if (Model.overlaySaveCover(statusJson)) {
        if (step === "ask" || step === "pin" || step === "minutes") step = "cover"
        if (!Model.askStillWaiting(askQueued, lastPending, pending)) askQueued = false
        lastPending = pending
      } else if (Model.overlayAskWaiting(statusJson)) {
        askQueued = false
        lastPending = pending
        if (step === "ask") step = "cover"
      } else if (!Model.askStillWaiting(askQueued, lastPending, pending)) {
        askQueued = false
        lastPending = pending
      } else {
        lastPending = pending
      }
    })
  }

  function advancePin() {
    var next = Model.overlayPinAdvance(pinDigits)
    if (!next.ok) return
    chosenMinutes = next.chosenMinutes
    step = next.step
  }

  function grant() {
    if (!Model.validPin(pinDigits)) return
    if (!Model.overlayGiveTimeOn(root.pinBusy)) return
    root.pinBusy = true
    var body = Model.pinGrantPayload(pinDigits, chosenMinutes * 60)
    overlayHTTP("POST", bankUrl() + "/v1/pin/grant", String(bank.askToken || ""), JSON.stringify(body), function(status, text) {
      if (status !== 200) {
        var next = Model.overlayPinFail(status, text)
        root.pinBusy = next.pinBusy
        applyPinDigits(next.pinDigits)
        root.pinWrong = next.pinWrong
        root.pinFailText = next.pinFailText
        root.step = next.step
        return
      }
      root.pinBusy = false
      applyPinDigits("")
      pinWrong = false
      pinFailText = "wrong pin"
      step = "cover"
      poll()
    })
  }

  function cancelPin() {
    if (!Model.overlayGiveTimeOn(root.pinBusy)) return
    var next = Model.overlayCancel()
    root.step = next.step
    applyPinDigits(next.pinDigits)
    root.pinWrong = next.pinWrong
    root.chosenMinutes = next.chosenMinutes
    root.pinBusy = false
  }

  function submitAsk() {
    if (root.waiting) return
    askQueued = true
    askWrong = false
    var body = Model.askPayload("fun", chosenMinutes * 60, "more time")
    overlayHTTP("POST", bankUrl() + "/v1/asks", String(bank.askToken || ""), JSON.stringify(body), function(status, text) {
      if (status !== 200) {
        askQueued = false
        askWrong = true
        askFailText = Model.askFailLabel()
        return
      }
      askWrong = false
      lastPending = Model.askWaitSeedPending(lastPending)
      step = "cover"
      poll()
    })
  }

  function helperPath(name) {
    return Qt.resolvedUrl("helpers/" + name).toString().replace(/^file:\/\//, "")
  }

  function splitHTTP(raw) {
    var s = String(raw || "")
    var i = s.lastIndexOf("\n")
    if (i < 0) return { status: 0, body: s }
    return { status: Number(s.slice(i + 1)) || 0, body: s.slice(0, i) }
  }

  function overlayHTTP(method, url, token, body, cb) {
    var q = (httpQueue || []).slice()
    q.push({ method: method, url: url, token: token || "", body: body || "", cb: cb })
    httpQueue = q
    pumpHTTP()
  }

  function pumpHTTP() {
    if (httpProc.running || !httpQueue || httpQueue.length === 0) return
    var q = httpQueue.slice()
    var job = q.shift()
    httpQueue = q
    httpJob = job
    httpBuf = ""
    httpProc.command = ["/usr/bin/bash", helperPath("loopback-http.sh"), job.method, job.url]
    httpProc.running = true
  }

  function hypr(batch) {
    hyprProc.command = ["/usr/bin/hyprctl", "--batch", batch]
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
  }

  function leaveKidProof() {
    if (submapOn) {
      hypr("dispatch submap reset")
      submapOn = false
    }
  }

  onBlocksSuperChanged: {
    if (blocksSuper) enterKidProof()
    else leaveKidProof()
  }

  Component.onDestruction: leaveKidProof()

  FileView {
    id: roleFile
    path: Quickshell.env("HOME") + "/.local/share/kidtimer/role"
    watchChanges: true
    printErrors: false
    preload: false
    blockAllReads: true
    onFileChanged: roleRead.running = true
    Component.onCompleted: roleRead.running = true
  }

  Timer {
    interval: 400
    repeat: true
    running: root.role === ""
    onTriggered: roleRead.running = true
  }

  Timer {
    interval: 400
    repeat: true
    running: root.role === "kid" && !String(bank.readToken || "")
    onTriggered: bankRead.running = true
  }

  FileView {
    id: bankFile
    path: Quickshell.env("HOME") + "/.local/share/kidtimer/kid-bar.json"
    watchChanges: true
    printErrors: false
    preload: false
    blockAllReads: true
    onFileChanged: bankRead.running = true
    Component.onCompleted: bankRead.running = true
  }

  Process {
    id: httpProc
    stdinEnabled: true
    stdout: SplitParser {
      splitMarker: ""
      onRead: function(data) { root.httpBuf += String(data || "") }
    }
    onStarted: {
      var job = root.httpJob || {}
      write(String(job.token || "") + "\n" + String(job.body || ""))
      stdinEnabled = false
    }
    onExited: {
      var job = root.httpJob
      root.httpJob = null
      stdinEnabled = true
      var parsed = root.splitHTTP(root.httpBuf)
      root.httpBuf = ""
      if (job && job.cb) job.cb(parsed.status, parsed.body)
      Qt.callLater(root.pumpHTTP)
    }
  }

  Process {
    id: roleRead
    command: ["/usr/bin/python3", "-I", "-S", helperPath("state.py"), "read", "role"]
    stdout: SplitParser {
      splitMarker: ""
      onRead: function(data) { root.applyRole(data) }
    }
  }

  Process {
    id: bankRead
    command: ["/usr/bin/python3", "-I", "-S", helperPath("state.py"), "read", "kid-bar.json"]
    stdout: SplitParser {
      splitMarker: ""
      onRead: function(data) { root.loadBank(data) }
    }
  }

  Timer {
    interval: 1000
    running: root.role === "kid"
    repeat: true
    triggeredOnStart: true
    onTriggered: root.poll()
  }

  Process { id: hyprProc }

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
          textFormat: Text.PlainText
          visible: root.step === "cover" && root.face === "save"
          width: parent.width
          horizontalAlignment: Text.AlignHCenter
          text: "save now"
          color: root.ink
          font.family: root.plex
          font.pixelSize: 28
          font.bold: true
        }

        Text {
          textFormat: Text.PlainText
          visible: root.step === "cover" && root.face === "save"
          width: parent.width
          horizontalAlignment: Text.AlignHCenter
          text: String(Model.saveSeconds(root.statusJson))
          color: root.ink
          font.family: root.plex
          font.pixelSize: 72
          font.bold: true
        }

        Text {
          textFormat: Text.PlainText
          visible: root.step === "cover" && root.face === "save"
          width: parent.width
          horizontalAlignment: Text.AlignHCenter
          text: "Super still works"
          color: root.ink
          font.family: root.plex
          font.pixelSize: 16
        }

        Rectangle {
          visible: root.step === "cover" && root.face === "save"
          width: parent.width
          height: 36
          color: "transparent"
          border.width: 1
          border.color: root.ink
          Text {
            textFormat: Text.PlainText
            anchors.centerIn: parent
            text: "Dismiss"
            color: root.ink
            font.family: root.plex
            font.pixelSize: 14
            font.bold: true
          }
          MouseArea {
            anchors.fill: parent
            cursorShape: Qt.PointingHandCursor
            onClicked: root.saveDismissed = true
          }
        }

        Text {
          textFormat: Text.PlainText
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
          textFormat: Text.PlainText
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
            textFormat: Text.PlainText
            anchors.centerIn: parent
            text: "lights out"
            color: Qt.rgba(root.ink.r, root.ink.g, root.ink.b, 0.7)
            font.family: root.plex
            font.pixelSize: 11
            font.capitalization: Font.AllUppercase
          }
        }

        Text {
          textFormat: Text.PlainText
          visible: root.step === "cover" && root.face === "bedtime"
          width: parent.width
          horizontalAlignment: Text.AlignHCenter
          text: "until " + Model.bedtimeEnd(root.statusJson)
          color: root.ink
          font.family: root.plex
          font.pixelSize: 16
        }

        Text {
          textFormat: Text.PlainText
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
          visible: root.step === "cover" && root.face !== "save"
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
              textFormat: Text.PlainText
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
                root.askWrong = false
                root.askFailText = Model.askFailLabel()
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
              textFormat: Text.PlainText
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
                root.pinFailText = "wrong pin"
                applyPinDigits("")
                pinField.forceActiveFocus()
              }
            }
          }
        }

        Text {
          textFormat: Text.PlainText
          visible: root.step === "pin"
          width: parent.width
          horizontalAlignment: Text.AlignHCenter
          text: root.pinWrong ? root.pinFailText : "Parent Pin"
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
          Keys.onReturnPressed: root.advancePin()
        }

        Rectangle {
          visible: root.step === "pin"
          width: parent.width
          height: 36
          color: "transparent"
          border.width: 1
          border.color: root.ink
          opacity: Model.validPin(root.pinDigits) ? 1 : 0.55
          Text {
            textFormat: Text.PlainText
            anchors.centerIn: parent
            text: "OK"
            color: root.ink
            font.family: root.plex
            font.pixelSize: 14
            font.bold: true
          }
          MouseArea {
            anchors.fill: parent
            enabled: Model.validPin(root.pinDigits)
            cursorShape: enabled ? Qt.PointingHandCursor : Qt.ArrowCursor
            onClicked: root.advancePin()
          }
        }

        Rectangle {
          visible: root.step === "pin"
          width: parent.width
          height: 36
          color: "transparent"
          border.width: 1
          border.color: root.ink
          Text {
            textFormat: Text.PlainText
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
            onClicked: root.cancelPin()
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
            opacity: (Model.overlayPinMinusOn(root.chosenMinutes) && Model.overlayGiveTimeOn(root.pinBusy)) ? 1 : 0.55
            Text {
              textFormat: Text.PlainText
              anchors.centerIn: parent
              text: "−5"
              color: root.ink
              font.family: root.plex
              font.pixelSize: 16
            }
            MouseArea {
              anchors.fill: parent
              enabled: Model.overlayPinMinusOn(root.chosenMinutes) && Model.overlayGiveTimeOn(root.pinBusy)
              cursorShape: enabled ? Qt.PointingHandCursor : Qt.ArrowCursor
              onClicked: root.chosenMinutes = Model.nudgeAskMinutes(root.chosenMinutes, -5)
            }
          }
          Column {
            anchors.centerIn: parent
            Text {
              textFormat: Text.PlainText
              anchors.horizontalCenter: parent.horizontalCenter
              text: Model.overlayStepperLabel(root.chosenMinutes)
              color: root.ink
              font.family: root.plex
              font.pixelSize: 28
              font.bold: true
            }
            Text {
              textFormat: Text.PlainText
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
            opacity: (Model.overlayPinPlusOn(root.chosenMinutes) && Model.overlayGiveTimeOn(root.pinBusy)) ? 1 : 0.55
            Text {
              textFormat: Text.PlainText
              anchors.centerIn: parent
              text: "+5"
              color: root.ink
              font.family: root.plex
              font.pixelSize: 16
            }
            MouseArea {
              anchors.fill: parent
              enabled: Model.overlayPinPlusOn(root.chosenMinutes) && Model.overlayGiveTimeOn(root.pinBusy)
              cursorShape: enabled ? Qt.PointingHandCursor : Qt.ArrowCursor
              onClicked: root.chosenMinutes = Model.nudgeAskMinutes(root.chosenMinutes, 5)
            }
          }
        }

        Item {
          visible: root.step === "minutes"
          width: parent.width
          height: 36
          Rectangle {
            width: (parent.width - 8) / 2
            height: parent.height
            anchors.left: parent.left
            color: "transparent"
            border.width: 1
            border.color: root.ink
            opacity: Model.overlayGiveTimeOn(root.pinBusy) ? 1 : 0.55
            Text {
              textFormat: Text.PlainText
              anchors.centerIn: parent
              text: "Cancel"
              color: root.ink
              font.family: root.plex
              font.pixelSize: 14
              font.bold: true
            }
            MouseArea {
              anchors.fill: parent
              enabled: Model.overlayGiveTimeOn(root.pinBusy)
              cursorShape: enabled ? Qt.PointingHandCursor : Qt.ArrowCursor
              onClicked: root.cancelPin()
            }
          }
          Rectangle {
            width: (parent.width - 8) / 2
            height: parent.height
            anchors.right: parent.right
            color: "transparent"
            border.width: 1
            border.color: root.ink
            opacity: Model.overlayGiveTimeOn(root.pinBusy) ? 1 : 0.55
            Text {
              textFormat: Text.PlainText
              anchors.centerIn: parent
              text: "Give time"
              color: root.ink
              font.family: root.plex
              font.pixelSize: 14
              font.bold: true
            }
            MouseArea {
              anchors.fill: parent
              enabled: Model.overlayGiveTimeOn(root.pinBusy)
              cursorShape: enabled ? Qt.PointingHandCursor : Qt.ArrowCursor
              onClicked: root.grant()
            }
          }
        }

        Text {
          textFormat: Text.PlainText
          visible: root.step === "ask"
          width: parent.width
          horizontalAlignment: Text.AlignHCenter
          text: root.askWrong ? root.askFailText : "Ask for more"
          color: root.askWrong ? root.urgent : root.dim
          font.family: root.plex
          font.pixelSize: 14
          font.bold: true
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
            opacity: Model.overlayAskMinusOn(root.chosenMinutes) ? 1 : 0.55
            Text {
              textFormat: Text.PlainText
              anchors.centerIn: parent
              text: "−10"
              color: root.ink
              font.family: root.plex
              font.pixelSize: 16
            }
            MouseArea {
              anchors.fill: parent
              enabled: Model.overlayAskMinusOn(root.chosenMinutes)
              cursorShape: enabled ? Qt.PointingHandCursor : Qt.ArrowCursor
              onClicked: {
                root.askWrong = false
                root.chosenMinutes = Model.nudgeOverlayAskMinutes(root.chosenMinutes, -10)
              }
            }
          }
          Column {
            anchors.centerIn: parent
            Text {
              textFormat: Text.PlainText
              anchors.horizontalCenter: parent.horizontalCenter
              text: Model.overlayAskStepperLabel(root.chosenMinutes)
              color: root.ink
              font.family: root.plex
              font.pixelSize: 28
              font.bold: true
            }
            Text {
              textFormat: Text.PlainText
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
            opacity: Model.overlayAskPlusOn(root.chosenMinutes) ? 1 : 0.55
            Text {
              textFormat: Text.PlainText
              anchors.centerIn: parent
              text: "+10"
              color: root.ink
              font.family: root.plex
              font.pixelSize: 16
            }
            MouseArea {
              anchors.fill: parent
              enabled: Model.overlayAskPlusOn(root.chosenMinutes)
              cursorShape: enabled ? Qt.PointingHandCursor : Qt.ArrowCursor
              onClicked: {
                root.askWrong = false
                root.chosenMinutes = Model.nudgeOverlayAskMinutes(root.chosenMinutes, 10)
              }
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
              textFormat: Text.PlainText
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
                root.askWrong = false
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
            opacity: root.waiting ? 0.55 : 1
            Text {
              textFormat: Text.PlainText
              anchors.centerIn: parent
              text: "Ask"
              color: root.ink
              font.family: root.plex
              font.pixelSize: 14
              font.bold: true
            }
            MouseArea {
              anchors.fill: parent
              enabled: !root.waiting
              cursorShape: enabled ? Qt.PointingHandCursor : Qt.ArrowCursor
              onClicked: root.submitAsk()
            }
          }
        }
      }
    }
  }
}
