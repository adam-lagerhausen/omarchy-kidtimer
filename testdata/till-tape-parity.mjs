import { spawn } from "node:child_process"
import { createServer } from "node:http"
import { readFile, mkdir, writeFile } from "node:fs/promises"
import { existsSync } from "node:fs"
import { createRequire } from "node:module"
import path from "node:path"
import { fileURLToPath } from "node:url"
import { execFile } from "node:child_process"
import { promisify } from "node:util"

const execFileAsync = promisify(execFile)
const require = createRequire(import.meta.url)
const home = process.env.HOME || "/home/parent"
const pkg = require(path.join(home, ".local/share/mise/installs/npm-playwright/1.62.1/lib/node_modules/playwright/index.js"))
const { chromium } = pkg
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..")
const goldDir = path.join(root, "scratch/three-modes/shots")
const outDir = process.env.TILL_TAPE_OUT || "/tmp/till-tape-parity"
const port = Number(process.env.TILL_TAPE_PORT || 8769)

const states = [
  { name: "a", gold: "a.png", setup: async () => {} },
  {
    name: "a-ask",
    gold: "a-ask.png",
    setup: async (page) => {
      await page.locator("[data-act='pick']").click()
      await page.locator("[data-kid='alex']").click()
      await page.locator("[data-act='bell']").click()
    }
  },
  {
    name: "a-picker",
    gold: "a-picker.png",
    setup: async (page) => {
      await page.locator("[data-act='pick']").click()
    }
  },
  {
    name: "a-locked",
    gold: "a-locked.png",
    setup: async (page) => {
      await page.locator("[data-act='lock']").click()
    }
  },
  {
    name: "a-settings",
    gold: "a-settings.png",
    setup: async (page) => {
      await page.locator("[data-act='settings']").click()
    }
  },
  {
    name: "a-settings-alex",
    gold: "a-settings-alex.png",
    setup: async (page) => {
      await page.locator("[data-act='settings']").click()
      await page.locator("[data-act='pick']").click()
      await page.locator("[data-kid='alex']").click()
    }
  }
]

function mime(file) {
  if (file.endsWith(".html")) return "text/html; charset=utf-8"
  if (file.endsWith(".js")) return "text/javascript; charset=utf-8"
  if (file.endsWith(".css")) return "text/css; charset=utf-8"
  if (file.endsWith(".png")) return "image/png"
  if (file.endsWith(".jpg") || file.endsWith(".jpeg")) return "image/jpeg"
  if (file.endsWith(".svg")) return "image/svg+xml"
  return "application/octet-stream"
}

async function serve() {
  const server = createServer(async (req, res) => {
    const url = new URL(req.url || "/", "http://127.0.0.1")
    let rel = decodeURIComponent(url.pathname)
    if (rel.endsWith("/")) rel += "index.html"
    const file = path.join(root, rel)
    if (!file.startsWith(root)) {
      res.writeHead(403)
      res.end()
      return
    }
    try {
      const body = await readFile(file)
      res.writeHead(200, { "content-type": mime(file) })
      res.end(body)
    } catch (e) {
      res.writeHead(404)
      res.end("missing")
    }
  })
  await new Promise((resolve, reject) => {
    server.once("error", reject)
    server.listen(port, "127.0.0.1", resolve)
  })
  return server
}

function compare(a, b, diff) {
  return new Promise((resolve) => {
    const child = spawn("compare", ["-metric", "AE", a, b, diff], { stdio: ["ignore", "pipe", "pipe"] })
    let err = ""
    child.stderr.on("data", (d) => { err += d })
    child.on("close", (code) => {
      const n = Number(String(err).trim().split(" ")[0])
      resolve({ ae: Number.isFinite(n) ? n : -1, code, raw: err.trim() })
    })
  })
}

async function main() {
  await mkdir(path.join(outDir, "html"), { recursive: true })
  await mkdir(path.join(outDir, "diff"), { recursive: true })
  const server = await serve()
  const browser = await chromium.launch({
    executablePath: "/usr/bin/chromium",
    args: ["--no-sandbox"]
  })
  const rows = []
  for (const state of states) {
    const page = await browser.newPage({ viewport: { width: 1440, height: 900 } })
    await page.goto("http://127.0.0.1:" + port + "/scratch/three-modes/a/index.html", { waitUntil: "networkidle" })
    await page.waitForTimeout(400)
    await state.setup(page)
    await page.waitForTimeout(200)
    const got = path.join(outDir, "html", state.name + ".png")
    await page.locator(".panel").screenshot({ path: got })
    const gold = path.join(goldDir, state.gold)
    const diff = path.join(outDir, "diff", state.name + ".png")
    let row = { state: state.name, gold: state.gold, got, ae: null, match: false }
    if (existsSync(gold)) {
      const c = await compare(gold, got, diff)
      row.ae = c.ae
      row.match = c.ae === 0
      row.compare = c.raw
    }
    rows.push(row)
    console.log(state.name, row.match ? "match" : "delta " + row.ae)
    await page.close()
  }
  await browser.close()
  server.close()
  await writeFile(path.join(outDir, "html-vs-gold.json"), JSON.stringify(rows, null, 2) + "\n")
  const failed = rows.filter((r) => !r.match)
  if (failed.length) {
    console.log("html vs locked goldens: " + failed.length + " nonzero")
    process.exitCode = 2
  } else {
    console.log("html vs locked goldens: all zero")
  }

  if (process.argv.includes("--qml")) {
    await mkdir(path.join(outDir, "qml"), { recursive: true })
    await mkdir(path.join(outDir, "qml-diff"), { recursive: true })
    const qmlRows = []
    for (const state of states) {
      const got = path.join(outDir, "qml", state.name + ".png")
      const host = path.join(root, "plugin-parent/till-tape-host.qml")
      try {
        await execFileAsync("qml6", [host, "--state", state.name, "--out", got], {
          timeout: 20000,
          env: { ...process.env, QT_QPA_PLATFORM: process.env.QT_QPA_PLATFORM || "offscreen" }
        })
      } catch (e) {
        console.log("qml", state.name, "fail", e.stderr || e.message)
        qmlRows.push({ state: state.name, error: String(e.stderr || e.message) })
        continue
      }
      const html = path.join(outDir, "html", state.name + ".png")
      const diff = path.join(outDir, "qml-diff", state.name + ".png")
      const c = existsSync(html) && existsSync(got) ? await compare(html, got, diff) : { ae: -1, raw: "missing" }
      qmlRows.push({ state: state.name, got, ae: c.ae, compare: c.raw })
      console.log("qml", state.name, "delta", c.ae)
    }
    await writeFile(path.join(outDir, "qml-vs-html.json"), JSON.stringify(qmlRows, null, 2) + "\n")
  }
}

main().catch((e) => {
  console.error(e)
  process.exit(1)
})
