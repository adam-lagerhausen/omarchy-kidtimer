import QtQuick
import QtQuick.Window
import "ParentModel.js" as Model

Window {
  id: win
  color: "transparent"
  flags: Qt.FramelessWindowHint | Qt.WindowStaysOnTopHint
  visible: true
  width: paper.width
  height: paper.height
  title: "till-tape-host"

  property string stateName: "a"
  property string outPath: ""

  function chromeFor(name) {
    if (name === "a-settings" || name === "a-settings-alex") return Model.chromeSettings()
    if (name === "a-picker") return { face: "home", picker: true, bell: false }
    if (name === "a-ask") return { face: "home", picker: false, bell: true }
    return Model.chromeHome()
  }

  function tapeFor(name) {
    var kid = (name === "a-ask" || name === "a-settings-alex") ? "alex" : "sam"
    var extra = {}
    if (name === "a-locked") extra.locked = true
    extra.now = Model.FIXTURE_NOW
    return Model.fixtureTape(kid, chromeFor(name), extra)
  }

  Tape {
    id: paper
    tape: win.tapeFor(win.stateName)
  }

  Component.onCompleted: {
    var args = Qt.application.arguments
    for (var i = 0; i < args.length; i++) {
      if (args[i] === "--state" && args[i + 1]) win.stateName = args[i + 1]
      if (args[i] === "--out" && args[i + 1]) win.outPath = args[i + 1]
    }
    paper.tape = win.tapeFor(win.stateName)
    win.width = paper.width
    win.height = Math.max(1, paper.implicitHeight)
    if (win.outPath) {
      Qt.callLater(function () {
        Qt.callLater(function () {
          paper.grabToImage(function (res) {
            res.saveToFile(win.outPath)
            Qt.quit()
          }, Qt.size(paper.width, Math.max(1, paper.implicitHeight)))
        })
      })
    }
  }
}
