import QtQuick
import qs.Commons
import "ParentModel.js" as Model

Rectangle {
  id: root
  property var tape: null
  property var bar: null
  property string pinDraft: ""
  property int pinCaret: 0
  property bool pinTouched: false
  property bool pinCommitted: false
  signal act(var ev)

  readonly property var t: tape || ({
    waiting: false,
    needsPin: false,
    howTo: null,
    chrome: { face: "home", picker: false, bell: false },
    kid: {
      name: "", nameUp: "", locked: false,
      face: { live: false, caption: "", coral: false },
      usedLabel: "0m",
      fun: { usedLabel: "0m", leftLabel: "0m LEFT", fillPct: 0, empty: true, barLow: true },
      policy: { bedLabel: "", upLabel: "", funDayRows: [] }
    },
    kids: [], asks: [], bellCount: 0,
    track: { beds: [], blocks: [], needle: null, log: [], hours: ["0", "6", "12", "18", "24"] },
    showLock: true, showStamp: false, lockLabel: "Lock", pinSet: false, lockArmed: false, ours: true
  })
  readonly property bool settingsOn: t.chrome.face === "settings"
  readonly property bool waitingHome: !!t.waiting
  readonly property bool needsPin: !!t.needsPin
  readonly property bool liveHome: !waitingHome && !needsPin
  readonly property bool ours: t.ours !== false
  readonly property bool controlsOn: liveHome && ours
  readonly property var howTo: t.howTo || { title: "", lines: [] }
  readonly property var adopt: t.adopt || null
  readonly property bool coralKid: !!t.kid.face.coral
  readonly property bool searchFocused: (settingsOn || needsPin) && !pinCommitted && pinTrap.activeFocus
  readonly property bool pinReady: Model.validPin(root.pinDraft) && !root.pinCommitted
  readonly property bool tapePinSet: !!t.pinSet

  readonly property color foreground: bar ? bar.foreground : Color.popups.text
  readonly property color urgent: bar ? bar.urgent : Color.urgent
  readonly property color accent: Color.accent
  readonly property color muted: Color.muted
  readonly property color surface: Color.popups.background
  readonly property color ink: coralKid ? urgent : foreground
  readonly property color quiet: {
    if (contrastRatio(muted, surface) >= 3)
      return muted
    if (colorLuminance(surface) < 0.5)
      return Qt.darker(foreground, 1.4)
    return Qt.rgba(
      foreground.r * 0.45 + surface.r * 0.55,
      foreground.g * 0.45 + surface.g * 0.55,
      foreground.b * 0.45 + surface.b * 0.55,
      1
    )
  }
  readonly property color wash: Style.normalFillFor(foreground, accent)
  readonly property color pick: Style.selectedFillFor(foreground, accent)
  readonly property color hatch: Util.alpha(foreground, 0.18)

  implicitWidth: 340
  width: implicitWidth
  color: "transparent"
  implicitHeight: inner.implicitHeight + 14 + 18

  FontLoader { id: plexReg; source: Qt.resolvedUrl("fonts/IBMPlexMono-Regular.ttf") }
  FontLoader { id: plexMed; source: Qt.resolvedUrl("fonts/IBMPlexMono-Medium.ttf") }
  FontLoader { id: plexSemi; source: Qt.resolvedUrl("fonts/IBMPlexMono-SemiBold.ttf") }

  readonly property string plex: plexReg.status === FontLoader.Ready ? plexReg.name : "IBM Plex Mono"

  onTapePinSetChanged: syncPinCommitted()
  onSettingsOnChanged: {
    syncPinCommitted()
    if ((settingsOn || needsPin) && !pinCommitted) Qt.callLater(focusPinTrap)
  }
  onNeedsPinChanged: {
    if (needsPin && !pinCommitted) Qt.callLater(focusPinTrap)
  }
  Component.onCompleted: {
    syncPinCommitted()
    if ((settingsOn || needsPin) && !pinCommitted) Qt.callLater(focusPinTrap)
  }

  function syncPinCommitted() {
    if (pinTouched) return
    pinCommitted = !!t.pinSet
  }

  function focusPinTrap() {
    if (pinCommitted) return
    pinTrap.forceActiveFocus()
  }

  function pinDigitsOnly(value) {
    var out = ""
    var src = String(value || "")
    for (var i = 0; i < src.length && out.length < 4; i++) {
      var c = src.charAt(i)
      if (c >= "0" && c <= "9") out += c
    }
    return out
  }

  function applyPinDraft(next) {
    var s = pinDigitsOnly(next)
    pinDraft = s
    pinCaret = s.length
    if (pinTrap.text !== s) pinTrap.text = s
  }

  function jumpPin(index) {
    if (pinCommitted) return
    pinTouched = true
    var n = Math.min(index, pinDraft.length)
    if (n < 0) n = 0
    applyPinDraft(pinDraft.slice(0, n))
    focusPinTrap()
  }

  function commitPin() {
    if (!pinReady) return
    var pin = pinDraft
    pinTouched = true
    pinCommitted = true
    applyPinDraft("")
    root.act({ kind: "pinSet", pin: pin })
  }

  function startOverPin() {
    pinTouched = true
    pinCommitted = false
    applyPinDraft("")
    focusPinTrap()
  }

  function colorChannelLuminance(value) {
    var channel = Number(value)
    if (!isFinite(channel)) return 0
    return channel <= 0.03928 ? channel / 12.92 : Math.pow((channel + 0.055) / 1.055, 2.4)
  }

  function colorLuminance(c) {
    return 0.2126 * colorChannelLuminance(c.r)
      + 0.7152 * colorChannelLuminance(c.g)
      + 0.0722 * colorChannelLuminance(c.b)
  }

  function contrastRatio(a, b) {
    var la = colorLuminance(a)
    var lb = colorLuminance(b)
    var hi = Math.max(la, lb)
    var lo = Math.min(la, lb)
    return (hi + 0.05) / (lo + 0.05)
  }

  Column {
    id: inner
    x: 18
    y: 14
    width: parent.width - 36
    spacing: 0

    Item {
      visible: liveHome
      width: parent.width
      height: 28
      SquareBtn {
        id: bellBtn
        width: 28
        height: 28
        danger: t.bellCount > 0
        onClicked: root.act({ kind: "bell" })
        TintIcon {
          anchors.centerIn: parent
          kind: "bell"
          tint: bellBtn.contentColor
        }
        Rectangle {
          visible: t.bellCount > 0
          anchors.right: parent.right
          anchors.top: parent.top
          anchors.rightMargin: -5
          anchors.topMargin: -5
          width: Math.max(14, bellN.implicitWidth + 6)
          height: 14
          color: urgent
          Text {
            id: bellN
            anchors.centerIn: parent
            text: String(t.bellCount)
            color: surface
            font.family: root.plex
            font.pixelSize: 10
            font.weight: Font.Bold
          }
        }
      }
      SquareBtn {
        id: gearBtn
        anchors.right: parent.right
        width: 28
        height: 28
        filled: settingsOn
        onClicked: root.act({ kind: "settings" })
        TintIcon {
          anchors.centerIn: parent
          kind: "gear"
          tint: gearBtn.contentColor
        }
      }
    }

    DashedRule { visible: liveHome; width: parent.width; topPad: 10; bottomPad: 10 }

    Column {
      width: parent.width
      visible: waitingHome
      spacing: 8
      Text {
        width: parent.width
        wrapMode: Text.WordWrap
        text: howTo.title
        color: ink
        font.family: root.plex
        font.pixelSize: 16
        font.weight: Font.DemiBold
      }
      Repeater {
        model: howTo.lines
        Column {
          required property var modelData
          width: inner.width
          spacing: 0
          Text {
            visible: !modelData.cmd
            width: parent.width
            wrapMode: Text.WordWrap
            text: modelData.text
            color: quiet
            font.family: root.plex
            font.pixelSize: 12
          }
          Rectangle {
            visible: !!modelData.cmd
            width: parent.width
            implicitHeight: cmdText.implicitHeight + 16
            height: implicitHeight
            color: "transparent"
            border.width: 1
            border.color: foreground
            Text {
              id: cmdText
              x: 10
              y: 8
              width: parent.width - 20
              wrapMode: Text.WrapAnywhere
              text: modelData.text
              color: foreground
              font.family: root.plex
              font.pixelSize: 11
            }
          }
        }
      }
    }

    Row {
      id: soldRow
      visible: liveHome
      width: parent.width
      spacing: 8
      height: Math.max(pickBox.implicitHeight, 36)

      SquareBtn {
        id: pickBox
        width: parent.width - (t.showLock ? 36 + 8 : 0)
        implicitHeight: pickCol.implicitHeight + 12
        height: implicitHeight
        picked: t.chrome.picker
        ink: root.ink
        onClicked: root.act({ kind: "pick" })
        Column {
          id: pickCol
          x: 10
          y: 6
          width: parent.width - 28
          spacing: 2
          Row {
            spacing: 6
            Rectangle {
              width: 7
              height: 7
              anchors.verticalCenter: parent.verticalCenter
              color: t.kid.face.live ? accent : urgent
            }
            Text {
              text: t.kid.nameUp
              color: ink
              font.family: root.plex
              font.pixelSize: 16
              font.weight: Font.DemiBold
            }
          }
          Text {
            text: t.kid.face.caption
            color: coralKid ? urgent : quiet
            font.family: root.plex
            font.pixelSize: 11
          }
        }
        Canvas {
          anchors.right: parent.right
          anchors.rightMargin: 10
          anchors.verticalCenter: parent.verticalCenter
          width: 8
          height: 5
          property color tip: ink
          property bool open: t.chrome.picker
          onPaint: {
            var ctx = getContext("2d")
            ctx.reset()
            ctx.fillStyle = tip
            ctx.beginPath()
            if (open) {
              ctx.moveTo(0, 5)
              ctx.lineTo(8, 5)
              ctx.lineTo(4, 0)
            } else {
              ctx.moveTo(0, 0)
              ctx.lineTo(8, 0)
              ctx.lineTo(4, 5)
            }
            ctx.closePath()
            ctx.fill()
          }
          onTipChanged: requestPaint()
          onOpenChanged: requestPaint()
        }
      }

      SquareBtn {
        id: lockBtn
        visible: t.showLock
        width: 36
        height: pickBox.height
        opacity: t.lockArmed ? 1 : 0.35
        filled: t.kid.locked
        danger: t.kid.locked
        onClicked: root.act({ kind: "lock" })
        TintIcon {
          anchors.centerIn: parent
          kind: t.kid.locked ? "lock-closed" : "lock-open"
          tint: lockBtn.contentColor
        }
      }
    }

    Column {
      width: parent.width
      visible: t.chrome.picker && liveHome
      topPadding: 8
      Rectangle {
        width: parent.width
        implicitHeight: pickerCol.implicitHeight + 8
        height: implicitHeight
        color: "transparent"
        border.width: 1
        border.color: foreground
        Column {
          id: pickerCol
          y: 4
          width: parent.width
          Repeater {
            model: t.kids
            SquareBtn {
              required property var modelData
              width: pickerCol.width
              height: pickLabel.implicitHeight + 4
              line: 0
              onClicked: root.act({ kind: "select", kidIndex: modelData.index })
              Text {
                id: pickLabel
                width: parent.width
                leftPadding: 10
                rightPadding: 10
                height: parent.height
                verticalAlignment: Text.AlignVCenter
                text: (modelData.face.live ? "● " : "○ ") + modelData.nameUp + "  " + modelData.face.caption
                color: foreground
                font.family: root.plex
                font.pixelSize: 12
              }
            }
          }
        }
      }
    }

    Column {
      width: parent.width
      visible: liveHome && !!adopt
      spacing: 8
      topPadding: 8
      Rectangle {
        width: parent.width
        implicitHeight: adoptCol.implicitHeight + 16
        height: implicitHeight
        color: "transparent"
        border.width: 1
        border.color: foreground
        Column {
          id: adoptCol
          x: 10
          y: 8
          width: parent.width - 20
          spacing: 6
          Text {
            width: parent.width
            wrapMode: Text.WordWrap
            text: adopt ? adopt.title : ""
            color: foreground
            font.family: root.plex
            font.pixelSize: 12
            font.weight: Font.DemiBold
          }
          Text {
            width: parent.width
            wrapMode: Text.WordWrap
            text: adopt ? adopt.body : ""
            color: quiet
            font.family: root.plex
            font.pixelSize: 12
          }
          Row {
            width: parent.width
            spacing: 8
            Repeater {
              model: [
                { kind: "adoptNo", label: "No" },
                { kind: "adoptYes", label: "Yes" }
              ]
              SquareBtn {
                required property var modelData
                width: (adoptCol.width - 8) / 2
                height: 24
                onClicked: root.act({ kind: modelData.kind })
                Text {
                  anchors.centerIn: parent
                  text: modelData.label
                  color: parent.contentColor
                  font.family: root.plex
                  font.pixelSize: 11
                }
              }
            }
          }
        }
      }
    }

    Column {
      width: parent.width
      visible: t.chrome.bell && liveHome && t.asks.length > 0
      spacing: 8
      topPadding: 8
      Repeater {
        model: t.asks
        Rectangle {
          required property var modelData
          width: inner.width
          implicitHeight: askCol.implicitHeight + 16
          height: implicitHeight
          color: "transparent"
          border.width: 1
          border.color: urgent
          Column {
            id: askCol
            x: 10
            y: 8
            width: parent.width - 20
            spacing: 6
            Text {
              width: parent.width
              wrapMode: Text.WordWrap
              text: modelData.text
              color: foreground
              font.family: root.plex
              font.pixelSize: 12
            }
            Row {
              width: parent.width
              spacing: 8
              Repeater {
                model: [
                  { kind: "deny", label: "DENY", ask: modelData },
                  { kind: "approve", label: "APPROVE", ask: modelData }
                ]
                SquareBtn {
                  required property var modelData
                  width: (askCol.width - 8) / 2
                  height: 24
                  onClicked: root.act({ kind: modelData.kind, ask: modelData.ask })
                  Text {
                    anchors.centerIn: parent
                    text: modelData.label
                    color: parent.contentColor
                    font.family: root.plex
                    font.pixelSize: 11
                  }
                }
              }
            }
          }
        }
      }
    }

    DashedRule { visible: controlsOn; width: parent.width; topPad: 10; bottomPad: 10 }

    Column {
      width: parent.width
      visible: !settingsOn && controlsOn
      spacing: 0
      Item {
        width: parent.width
        height: 22
        Text {
          anchors.left: parent.left
          anchors.verticalCenter: parent.verticalCenter
          text: "USED"
          color: quiet
          font.family: root.plex
          font.pixelSize: 13
        }
        Text {
          anchors.right: parent.right
          anchors.verticalCenter: parent.verticalCenter
          text: t.kid.fun.usedLabel
          color: foreground
          font.family: root.plex
          font.pixelSize: 13
        }
      }
      Item { width: 1; height: 6 }
      Item {
        width: parent.width
        height: 24
        opacity: t.kid.locked ? 0.35 : 1
        SquareBtn {
          width: 36
          height: 24
          enabled: !t.kid.locked && !t.kid.fun.empty
          onClicked: root.act({ kind: "minus10" })
          Text {
            anchors.centerIn: parent
            text: "−10"
            color: parent.contentColor
            font.family: root.plex
            font.pixelSize: 11
          }
        }
        Text {
          anchors.centerIn: parent
          text: t.kid.fun.leftLabel
          color: t.kid.fun.empty ? urgent : foreground
          font.family: root.plex
          font.pixelSize: 13
          font.weight: Font.DemiBold
        }
        SquareBtn {
          anchors.right: parent.right
          width: 36
          height: 24
          enabled: !t.kid.locked
          onClicked: root.act({ kind: "plus10" })
          Text {
            anchors.centerIn: parent
            text: "+10"
            color: parent.contentColor
            font.family: root.plex
            font.pixelSize: 11
          }
        }
      }
      Item { width: 1; height: 8 }
      Rectangle {
        width: parent.width
        height: 6
        color: wash
        border.width: 1
        border.color: foreground
        Item {
          anchors.fill: parent
          anchors.margins: 1
          Rectangle {
            width: Math.max(0, parent.width * (t.kid.fun.fillPct / 100))
            height: parent.height
            color: (t.kid.fun.empty || t.kid.fun.barLow) ? urgent : foreground
          }
        }
      }
    }

    Column {
      width: parent.width
      visible: needsPin || (settingsOn && liveHome)
      spacing: 0
      Text {
        text: Model.parentPinLabel()
        color: foreground
        font.family: root.plex
        font.pixelSize: 13
        font.weight: Font.DemiBold
        bottomPadding: 6
      }
      Text {
        width: parent.width
        wrapMode: Text.WordWrap
        text: Model.parentPinWhy()
        color: quiet
        font.family: root.plex
        font.pixelSize: 11
        lineHeight: 1.4
        lineHeightMode: Text.ProportionalHeight
        bottomPadding: 10
      }
      Item {
        width: inner.width
        height: 40
        MouseArea {
          anchors.fill: parent
          enabled: !root.pinCommitted
          onClicked: root.focusPinTrap()
        }
        Row {
          anchors.verticalCenter: parent.verticalCenter
          spacing: 6
          Repeater {
            model: 4
            Rectangle {
              id: pinSlot
              required property int index
              readonly property string kind: Model.pinSlotKind(root.pinDraft, index, root.pinCaret, root.pinCommitted)
              property bool blinkOn: true
              width: 28
              height: 28
              color: kind === "caret" ? foreground : surface
              border.width: 1
              border.color: foreground
              Text {
                visible: pinSlot.kind === "digit"
                anchors.centerIn: parent
                text: Model.pinBoxText(root.pinDraft, pinSlot.index)
                color: foreground
                font.family: root.plex
                font.pixelSize: 16
                font.weight: Font.Medium
              }
              Rectangle {
                visible: pinSlot.kind === "dot"
                anchors.centerIn: parent
                width: 6
                height: 6
                color: foreground
              }
              Rectangle {
                visible: pinSlot.kind === "caret" && pinSlot.blinkOn
                anchors.centerIn: parent
                width: 9
                height: 15
                color: surface
              }
              Timer {
                interval: 525
                running: pinSlot.kind === "caret"
                repeat: true
                onRunningChanged: if (running) pinSlot.blinkOn = true
                onTriggered: pinSlot.blinkOn = !pinSlot.blinkOn
              }
              MouseArea {
                anchors.fill: parent
                enabled: !root.pinCommitted
                cursorShape: Qt.IBeamCursor
                onClicked: root.jumpPin(pinSlot.index)
              }
            }
          }
        }
        SquareBtn {
          id: pinSetBtn
          anchors.right: parent.right
          anchors.verticalCenter: parent.verticalCenter
          width: Math.max(56, pinSetLabel.implicitWidth + 16)
          height: 28
          filled: root.pinReady
          enabled: root.pinReady || (!needsPin && root.pinCommitted)
          z: 1
          onClicked: {
            if (!needsPin && root.pinCommitted) root.startOverPin()
            else root.commitPin()
          }
          Text {
            id: pinSetLabel
            anchors.centerIn: parent
            text: (needsPin || !root.pinCommitted) ? "Set" : "Change"
            color: pinSetBtn.enabled ? pinSetBtn.contentColor : quiet
            font.family: root.plex
            font.pixelSize: 12
          }
        }
        TextInput {
          id: pinTrap
          width: 1
          height: 1
          opacity: 0
          color: foreground
          cursorVisible: false
          echoMode: TextInput.Normal
          inputMethodHints: Qt.ImhDigitsOnly
          maximumLength: 4
          enabled: !root.pinCommitted
          validator: RegularExpressionValidator { regularExpression: /[0-9]{0,4}/ }
          Keys.onReturnPressed: root.commitPin()
          Keys.onEnterPressed: root.commitPin()
          onTextChanged: {
            if (root.pinCommitted) {
              if (text !== "") text = ""
              return
            }
            var s = root.pinDigitsOnly(text)
            if (s !== text) {
              text = s
              return
            }
            root.pinTouched = true
            root.pinDraft = s
            root.pinCaret = s.length
          }
        }
      }
      Column {
        width: parent.width
        visible: settingsOn && !needsPin
        spacing: 0
        DashedRule { width: parent.width; topPad: 10; bottomPad: 8 }
        Repeater {
          model: [
            { kind: "bed", k: "BED", v: t.kid.policy.bedLabel },
            { kind: "up", k: "UP", v: t.kid.policy.upLabel }
          ]
        Item {
          id: clockRow
          required property var modelData
          required property int index
          width: inner.width
          height: 40
          Text {
            anchors.verticalCenter: parent.verticalCenter
            width: 34
            text: modelData.k
            color: quiet
            font.family: root.plex
            font.pixelSize: 11
          }
          SquareBtn {
            x: 42
            anchors.verticalCenter: parent.verticalCenter
            width: 28
            height: 28
            onClicked: root.act({ kind: clockRow.index === 0 ? "bed" : "up", delta: -15 })
            Text {
              anchors.centerIn: parent
              text: "−"
              color: parent.contentColor
              font.family: root.plex
              font.pixelSize: 16
            }
          }
          Text {
            anchors.horizontalCenter: parent.horizontalCenter
            anchors.verticalCenter: parent.verticalCenter
            text: modelData.v
            color: foreground
            font.family: root.plex
            font.pixelSize: 22
            font.weight: Font.DemiBold
            font.letterSpacing: -0.6
          }
          SquareBtn {
            anchors.right: parent.right
            anchors.verticalCenter: parent.verticalCenter
            width: 28
            height: 28
            onClicked: root.act({ kind: clockRow.index === 0 ? "bed" : "up", delta: 15 })
            Text {
              anchors.centerIn: parent
              text: "+"
              color: parent.contentColor
              font.family: root.plex
              font.pixelSize: 16
            }
          }
        }
      }
      }
    }

    Item { width: 1; height: liveHome && settingsOn ? 2 : (needsPin ? 0 : 8) }

    DayTrack {
      visible: controlsOn
      width: parent.width
      height: settingsOn ? 32 : 28
      track: t.track
      settingsFace: settingsOn
    }

    Item {
      visible: controlsOn
      width: parent.width
      height: 16
      Repeater {
        model: t.track.hours
        Text {
          required property var modelData
          required property int index
          text: modelData
          color: quiet
          font.family: root.plex
          font.pixelSize: 10
          x: index === 0 ? 0 : (index === 4 ? parent.width - implicitWidth : parent.width * index / 4 - implicitWidth / 2)
          anchors.verticalCenter: parent.verticalCenter
        }
      }
    }

    Column {
      width: parent.width
      visible: !settingsOn && controlsOn
      topPadding: 2
      Repeater {
        model: t.track.log
        Item {
          required property var modelData
          width: inner.width
          height: 18
          Text {
            anchors.verticalCenter: parent.verticalCenter
            text: modelData.clock + "  "
            color: foreground
            font.family: root.plex
            font.pixelSize: 12
            Text {
              anchors.left: parent.right
              anchors.verticalCenter: parent.verticalCenter
              text: modelData.name
              color: foreground
              font.family: root.plex
              font.pixelSize: 12
              font.weight: Font.Medium
            }
          }
          Text {
            anchors.right: parent.right
            anchors.verticalCenter: parent.verticalCenter
            text: modelData.dur
            color: foreground
            font.family: root.plex
            font.pixelSize: 12
          }
        }
      }
    }

    Column {
      width: parent.width
      visible: settingsOn && controlsOn
      spacing: 0
      DashedRule { width: parent.width; topPad: 10; bottomPad: 8 }
      Text {
        text: "TIME"
        color: quiet
        font.family: root.plex
        font.pixelSize: 13
        bottomPadding: 4
      }
      Repeater {
        model: t.kid.policy.funDayRows
        Item {
          required property var modelData
          width: inner.width
          height: 30
          Text {
            anchors.verticalCenter: parent.verticalCenter
            width: 42
            text: modelData.dow
            color: quiet
            font.family: root.plex
            font.pixelSize: 13
          }
          SquareBtn {
            x: 48
            anchors.verticalCenter: parent.verticalCenter
            width: 36
            height: 24
            onClicked: root.act({ kind: "funDay", day: modelData.day, delta: -15 })
            Text {
              anchors.centerIn: parent
              text: "−15"
              color: parent.contentColor
              font.family: root.plex
              font.pixelSize: 11
            }
          }
          Text {
            anchors.centerIn: parent
            text: modelData.label
            color: foreground
            font.family: root.plex
            font.pixelSize: 13
            font.weight: Font.DemiBold
          }
          SquareBtn {
            anchors.right: parent.right
            anchors.verticalCenter: parent.verticalCenter
            width: 36
            height: 24
            onClicked: root.act({ kind: "funDay", day: modelData.day, delta: 15 })
            Text {
              anchors.centerIn: parent
              text: "+15"
              color: parent.contentColor
              font.family: root.plex
              font.pixelSize: 11
            }
          }
        }
      }
    }
  }

  Item {
    visible: t.showStamp
    z: 5
    width: lockStampText.implicitWidth + 28
    height: lockStampText.implicitHeight + 16
    x: (root.width - width) / 2
    y: {
      var _layout = inner.implicitHeight
      var below = soldRow.mapToItem(root, soldRow.width / 2, soldRow.height)
      return below.y + 10
    }
    rotation: -12
    Rectangle {
      anchors.fill: parent
      border.width: 3
      border.color: urgent
      color: "transparent"
    }
    Text {
      id: lockStampText
      anchors.centerIn: parent
      text: "LOCKED"
      color: urgent
      font.family: root.plex
      font.pixelSize: 22
      font.weight: Font.DemiBold
      font.letterSpacing: 4.4
    }
  }

  component SquareBtn: Rectangle {
    id: btn
    property bool filled: false
    property bool picked: false
    property bool danger: false
    property int line: 1
    property color ink: root.foreground
    property color paper: root.surface
    property color accentColor: root.accent
    property color urgentColor: root.urgent
    readonly property color washInk: danger ? urgentColor : ink
    readonly property color washAccent: danger ? urgentColor : accentColor
    readonly property bool hot: mouse.containsMouse && enabled
    readonly property bool down: mouse.pressed && enabled
    readonly property color contentColor: filled ? paper : washInk
    signal clicked()

    radius: 0
    color: {
      if (filled) return danger ? urgentColor : ink
      if (picked) return root.pick
      if (!enabled) return "transparent"
      if (down) return Style.pressedFillFor(washInk, washAccent)
      if (hot) return Style.hoverFillFor(washInk, washAccent)
      return "transparent"
    }
    border.width: line
    border.color: danger ? urgentColor : ink

    MouseArea {
      id: mouse
      anchors.fill: parent
      z: 1
      hoverEnabled: true
      enabled: btn.enabled
      cursorShape: enabled ? Qt.PointingHandCursor : Qt.ArrowCursor
      onClicked: btn.clicked()
    }
  }

  component TintIcon: Canvas {
    id: glyph
    property string kind
    property color tint
    implicitWidth: 16
    implicitHeight: 16
    width: implicitWidth
    height: implicitHeight
    antialiasing: true
    onKindChanged: requestPaint()
    onTintChanged: requestPaint()
    onWidthChanged: requestPaint()
    onHeightChanged: requestPaint()
    onPaint: {
      var ctx = getContext("2d")
      ctx.reset()
      ctx.fillStyle = tint
      ctx.strokeStyle = tint
      ctx.scale(width / 16, height / 16)
      if (kind === "bell") {
        ctx.beginPath()
        ctx.arc(8, 2.55, 1.15, Math.PI, 0)
        ctx.fill()
        ctx.beginPath()
        ctx.moveTo(2.4, 12.1)
        ctx.lineTo(2.4, 7.3)
        ctx.bezierCurveTo(2.4, 4.15, 4.85, 2.55, 8, 2.55)
        ctx.bezierCurveTo(11.15, 2.55, 13.6, 4.15, 13.6, 7.3)
        ctx.lineTo(13.6, 12.1)
        ctx.lineTo(15.1, 13.55)
        ctx.lineTo(0.9, 13.55)
        ctx.closePath()
        ctx.fill()
        ctx.beginPath()
        ctx.arc(8, 14.55, 1.55, 0, Math.PI * 2)
        ctx.fill()
      } else if (kind === "gear") {
        ctx.translate(8, 8)
        var teeth = 8
        var step = Math.PI * 2 / teeth
        var outer = 7.35
        var inner = 5.05
        var hole = 2.55
        var i, a, a0, a1, a2, a3
        ctx.beginPath()
        for (i = 0; i < teeth; i++) {
          a = i * step - Math.PI / 2
          a0 = a - step * 0.22
          a1 = a - step * 0.12
          a2 = a + step * 0.12
          a3 = a + step * 0.22
          if (i === 0) ctx.moveTo(inner * Math.cos(a0), inner * Math.sin(a0))
          else ctx.lineTo(inner * Math.cos(a0), inner * Math.sin(a0))
          ctx.lineTo(outer * Math.cos(a1), outer * Math.sin(a1))
          ctx.lineTo(outer * Math.cos(a2), outer * Math.sin(a2))
          ctx.lineTo(inner * Math.cos(a3), inner * Math.sin(a3))
        }
        ctx.closePath()
        ctx.fill()
        ctx.globalCompositeOperation = "destination-out"
        ctx.beginPath()
        ctx.arc(0, 0, hole, 0, Math.PI * 2)
        ctx.fill()
      } else if (kind === "lock-open" || kind === "lock-closed") {
        ctx.lineWidth = 1.7
        ctx.lineCap = "round"
        ctx.lineJoin = "round"
        ctx.beginPath()
        if (kind === "lock-closed") {
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
        ctx.beginPath()
        ctx.moveTo(3.2, 7)
        ctx.lineTo(12.8, 7)
        ctx.arcTo(14, 7, 14, 8.2, 1.2)
        ctx.lineTo(14, 13.8)
        ctx.arcTo(14, 15, 12.8, 15, 1.2)
        ctx.lineTo(3.2, 15)
        ctx.arcTo(2, 15, 2, 13.8, 1.2)
        ctx.lineTo(2, 8.2)
        ctx.arcTo(2, 7, 3.2, 7, 1.2)
        ctx.closePath()
        ctx.fill()
      }
    }
  }

  component DashedRule: Item {
    property color color: root.foreground
    property int topPad: 10
    property int bottomPad: 10
    height: topPad + bottomPad + 1
    Canvas {
      y: topPad
      width: parent.width
      height: 1
      property color stroke: parent.color
      onPaint: {
        var ctx = getContext("2d")
        ctx.reset()
        ctx.strokeStyle = stroke
        ctx.lineWidth = 1
        ctx.setLineDash([3, 3])
        ctx.beginPath()
        ctx.moveTo(0, 0.5)
        ctx.lineTo(width, 0.5)
        ctx.stroke()
      }
      onWidthChanged: requestPaint()
      onStrokeChanged: requestPaint()
    }
  }

  component HatchBand: Canvas {
    property color hatchFill: root.hatch
    property color hatchInk: root.foreground
    property bool filled: false
    property bool invert: false
    property real inkAlpha: 0.55
    onPaint: {
      var ctx = getContext("2d")
      ctx.reset()
      if (filled) {
        ctx.fillStyle = hatchFill
        ctx.fillRect(0, 0, width, height)
      }
      ctx.strokeStyle = hatchInk
      ctx.globalAlpha = inkAlpha
      ctx.lineWidth = 1
      var s = 5
      for (var x = -height; x < width + height; x += s) {
        ctx.beginPath()
        if (invert) {
          ctx.moveTo(x, 0)
          ctx.lineTo(x + height, height)
        } else {
          ctx.moveTo(x, height)
          ctx.lineTo(x + height, 0)
        }
        ctx.stroke()
      }
    }
    onWidthChanged: requestPaint()
    onHeightChanged: requestPaint()
    onHatchFillChanged: requestPaint()
    onHatchInkChanged: requestPaint()
    onFilledChanged: requestPaint()
    onInvertChanged: requestPaint()
    onInkAlphaChanged: requestPaint()
  }

  component DayTrack: Item {
    id: dayTrack
    property var track: ({ beds: [], blocks: [], needle: null })
    property bool settingsFace: false

    function blockRight(b) {
      var left = width * Number(b.leftPct) / 100
      var right = left + width * Number(b.widthPct) / 100
      if (track.needle != null) {
        var cap = width * Number(track.needle) / 100
        if (right > cap) right = cap
      }
      return right
    }

    function blockLeft(b) {
      var left = width * Number(b.leftPct) / 100
      var right = blockRight(b)
      if (right - left < 4) left = right - 4
      if (left < 0) left = 0
      return left
    }

    function blockWidth(b) {
      return Math.max(0, blockRight(b) - blockLeft(b))
    }

    Rectangle {
      anchors.fill: parent
      color: root.wash
      border.width: 1
      border.color: root.foreground
    }
    Repeater {
      model: track.beds
      HatchBand {
        required property var modelData
        x: parent.width * modelData.leftPct / 100
        y: 1
        width: Math.max(1, parent.width * modelData.widthPct / 100)
        height: parent.height - 2
        filled: settingsFace
        inkAlpha: settingsFace ? 0.8 : 0.55
      }
    }
    Repeater {
      model: track.blocks
      Rectangle {
        required property var modelData
        x: dayTrack.blockLeft(modelData)
        y: 1
        width: dayTrack.blockWidth(modelData)
        height: parent.height - 2
        color: root.accent
      }
    }
    Rectangle {
      visible: track.needle != null
      x: parent.width * (Number(track.needle) / 100) - 1
      y: -3
      width: 2
      height: parent.height + 6
      color: root.foreground
    }
  }
}
