#!/usr/bin/env node
const fs = require("fs")
const path = require("path")
const vm = require("vm")

function assertEqual(got, want, msg) {
  const a = JSON.stringify(got)
  const b = JSON.stringify(want)
  if (a !== b) {
    console.error(msg + ": got " + a + " want " + b)
    process.exit(1)
  }
}

function makeEl(tag) {
  const el = {
    tagName: String(tag).toUpperCase(),
    className: "",
    textContent: "",
    type: "",
    disabled: false,
    children: [],
    listeners: {},
    appendChild(child) {
      this.children.push(child)
      return child
    },
    addEventListener(ev, fn) {
      if (!this.listeners[ev]) this.listeners[ev] = []
      this.listeners[ev].push(fn)
    },
    click() {
      const fns = this.listeners.click || []
      for (let i = 0; i < fns.length; i++) fns[i]({ type: "click" })
    }
  }
  return el
}

function loadPage(fetchImpl) {
  const html = fs.readFileSync(path.join(__dirname, "..", "daemon", "internal", "desk", "kitchen.html"), "utf8")
  const match = html.match(/<script>([\s\S]*?)<\/script>/)
  if (!match) {
    console.error("kitchen.html missing script")
    process.exit(1)
  }
  const root = {
    children: [],
    _text: "",
    get textContent() { return this._text },
    set textContent(v) {
      this._text = v
      if (v === "") this.children = []
    },
    appendChild(child) {
      this.children.push(child)
      return child
    }
  }
  const ctx = {
    document: {
      getElementById(id) { return id === "asks" ? root : null },
      createElement: makeEl
    },
    fetch: fetchImpl,
    setInterval() { return 0 },
    encodeURIComponent,
    JSON,
    Error,
    console
  }
  vm.runInNewContext(match[1], ctx)
  return { root, ctx }
}

function askText(root) {
  if (!root.children.length) return ""
  const who = root.children[0].children[0]
  return who ? who.textContent : ""
}

function buttons(root) {
  if (!root.children.length) return []
  const acts = root.children[0].children[1]
  return acts ? acts.children : []
}

const pendingAsk = { id: "a1", kid: "kid-1", kid_name: "Ada", seconds: 600, text: "Ada asked for 10 more minutes" }

function jsonOk(body) {
  return Promise.resolve({
    ok: true,
    json() { return Promise.resolve(body) }
  })
}

function jsonPost() {
  return Promise.resolve({ ok: true, json() { return Promise.resolve({}) } })
}

async function flush() {
  for (let i = 0; i < 20; i++) await Promise.resolve()
}

;(async function () {
  const first = {}
  first.p = new Promise((resolve) => { first.resolve = resolve })
  const second = {}
  second.p = new Promise((resolve) => { second.resolve = resolve })
  let asksCalls = 0
  const stale = loadPage(function (url) {
    if (url === "/v1/kitchen/asks") {
      asksCalls += 1
      if (asksCalls === 1) return first.p.then(() => jsonOk({ asks: [pendingAsk] }))
      if (asksCalls === 2) return second.p.then(() => jsonOk({ asks: [] }))
      return jsonOk({ asks: [] })
    }
    return jsonPost()
  })
  stale.ctx.load()
  second.resolve()
  await flush()
  assertEqual(askText(stale.root), "", "fast empty poll paints nothing")
  first.resolve()
  await flush()
  assertEqual(askText(stale.root), "", "stale GET with a decided card does not repaint")
  assertEqual(asksCalls, 2, "stale case used the first page load plus one poll")

  let remaining = 3600
  let posts = []
  let decideHold = null
  const live = loadPage(function (url, opts) {
    if (url === "/v1/kitchen/asks") {
      const asks = remaining === 3600 ? [pendingAsk] : []
      return jsonOk({ asks: asks })
    }
    if (opts && opts.method === "POST") {
      posts.push({ url: url, body: JSON.parse(opts.body) })
      return new Promise((resolve) => {
        decideHold = function () {
          remaining += 600
          resolve({ ok: true, json() { return Promise.resolve({}) } })
        }
      })
    }
    return jsonPost()
  })
  await flush()
  assertEqual(askText(live.root), "Ada asked for 10 more minutes", "kitchen card matches the ping")
  const btns = buttons(live.root)
  assertEqual(btns.map(b => b.textContent), ["DENY", "APPROVE"], "DENY and APPROVE on the kitchen card")
  btns[1].click()
  assertEqual(!!live.ctx.busy["kid-1/a1"], true, "approve marks the ask busy")
  btns[0].click()
  btns[1].click()
  assertEqual(posts.length, 1, "in-flight tap does not queue a second decide")
  assertEqual(posts[0].url, "/v1/kitchen/asks/kid-1/a1/decide", "approve posts the kitchen decide path")
  assertEqual(posts[0].body, { decision: "approve" }, "approve body")
  decideHold()
  await flush()
  assertEqual(remaining, 4200, "approve credits ten minutes without the panel")
  assertEqual(askText(live.root), "", "successful decide drops the card")

  console.log("ok")
})().catch(function (err) {
  console.error(err && err.stack ? err.stack : err)
  process.exit(1)
})
