/*
 * ray serve — a página do editor de templates.
 *
 * Sem framework e sem build. Todo dado que vem do servidor (nome, conteúdo,
 * mensagem de erro, texto do diff) entra no DOM só por textContent,
 * createElement e createTextNode: nunca vira HTML.
 *
 * Seções: funções puras · DOM · API e sessão · status · árvore · abas ·
 * editor · preview · painel · ações (salvar, restaurar, recarregar) · início.
 */
(function () {
  "use strict";

  /* ---------- Funções puras (sem DOM; o teste em node carrega estas) ---------- */

  // restoreEOL devolve o texto do editor (sempre LF) com o fim de linha que o
  // arquivo tinha: "crlf" troca cada \n por \r\n.
  function restoreEOL(text, eol) {
    return eol === "crlf" ? text.replace(/\r?\n/g, "\r\n") : text;
  }

  // As cores e a ordem dos estados são as do CLI.
  const STATE_ORDER = ["outdated", "new", "edited", "custom", "current"];

  // summarize conta os templates por estado: "1 outdated · 1 edited".
  function summarize(list) {
    const counts = {};
    for (const e of list) counts[e.state] = (counts[e.state] || 0) + 1;
    const known = STATE_ORDER.filter((s) => counts[s]);
    const other = Object.keys(counts).filter((s) => STATE_ORDER.indexOf(s) < 0).sort();
    return known.concat(other).map((s) => counts[s] + " " + s).join(" · ");
  }

  // buildTree agrupa a lista por pasta (o nome usa "/"): cada nó de pasta tem
  // {folder, path, children}; cada folha, {entry}. Pastas antes de arquivos.
  function buildTree(list) {
    const root = { folder: "", path: "", children: [] };
    const sorted = list.slice().sort((a, b) => (a.name < b.name ? -1 : a.name > b.name ? 1 : 0));
    for (const entry of sorted) {
      const parts = entry.name.split("/");
      let node = root;
      for (let i = 0; i < parts.length - 1; i++) {
        const path = parts.slice(0, i + 1).join("/");
        let next = node.children.find((c) => c.folder !== undefined && c.path === path);
        if (!next) {
          next = { folder: parts[i], path: path, children: [] };
          node.children.push(next);
        }
        node = next;
      }
      node.children.push({ entry: entry, label: parts[parts.length - 1] });
    }
    const order = (n) => {
      n.children.sort((a, b) => {
        const fa = a.folder !== undefined ? 0 : 1;
        const fb = b.folder !== undefined ? 0 : 1;
        if (fa !== fb) return fa - fb;
        const la = a.folder !== undefined ? a.folder : a.label;
        const lb = b.folder !== undefined ? b.folder : b.label;
        return la < lb ? -1 : la > lb ? 1 : 0;
      });
      n.children.forEach((c) => c.folder !== undefined && order(c));
    };
    order(root);
    return root;
  }

  // editedLines tira o N de "...: N lines differ from the default; ...".
  function editedLines(detalhe) {
    const m = /(\d+) lines? differ/.exec(detalhe || "");
    return m ? Number(m[1]) : 0;
  }

  // Realce de ações {{ }} de text/template. Um token é [texto, classe]; a
  // classe vazia é texto comum.
  const ACTION_TOKEN = new RegExp(
    [
      "(\\{\\{-?|-?\\}\\})", // delimitadores
      "(\"(?:[^\"\\\\]|\\\\.)*\"|`[^`]*`)", // strings
      "(\\.[A-Za-z_][\\w.]*|\\$[\\w]*)", // campos e variáveis
      "\\b(if|else|end|range|with|define|template|block|break|continue|and|or|not|eq|ne|lt|le|gt|ge|len|index|printf|print|println|slice|call|nil)\\b",
    ].join("|"),
    "g"
  );

  function tokenizeAction(text) {
    const out = [];
    let last = 0;
    ACTION_TOKEN.lastIndex = 0;
    let m;
    while ((m = ACTION_TOKEN.exec(text)) !== null) {
      if (m.index > last) out.push([text.slice(last, m.index), "tk-act"]);
      const cls = m[1] ? "tk-delim" : m[2] ? "tk-str" : m[3] ? "tk-field" : "tk-kw";
      out.push([m[0], cls]);
      last = m.index + m[0].length;
    }
    if (last < text.length) out.push([text.slice(last), "tk-act"]);
    return out;
  }

  // highlightLine quebra uma linha em tokens. mode é o que vem da linha
  // anterior: "" (texto), "act" (dentro de {{ }}) ou "comment" ({{/* */}}).
  function highlightLine(line, mode) {
    const out = [];
    let i = 0;
    while (i < line.length) {
      let from = i;
      if (!mode) {
        const j = line.indexOf("{{", i);
        if (j < 0) {
          out.push([line.slice(i), ""]);
          break;
        }
        if (j > i) out.push([line.slice(i, j), ""]);
        mode = /^-?\s*\/\*/.test(line.slice(j + 2)) ? "comment" : "act";
        i = j;
        from = j + 2;
      }
      const end = line.indexOf("}}", from);
      const stop = end < 0 ? line.length : end + 2;
      const piece = line.slice(i, stop);
      if (mode === "comment") out.push([piece, "tk-comment"]);
      else tokenizeAction(piece).forEach((t) => out.push(t));
      i = stop;
      if (end >= 0) mode = "";
    }
    return { tokens: out, mode: mode };
  }

  // diffLineClass escolhe a cor de uma linha de diff unificado.
  function diffLineClass(line) {
    if (line.startsWith("+++") || line.startsWith("---") || line.startsWith("diff ") || line.startsWith("index ")) return "d-meta";
    if (line.startsWith("@@")) return "d-hunk";
    if (line.startsWith("+")) return "d-add";
    if (line.startsWith("-")) return "d-del";
    return "d-ctx";
  }

  const pure = { restoreEOL, summarize, buildTree, editedLines, highlightLine, diffLineClass };
  if (typeof module !== "undefined" && module.exports) {
    module.exports = pure;
    return;
  }

  /* ---------- DOM ---------- */

  const $ = (id) => document.getElementById(id);

  // el cria um elemento: props "class", "text", "on" (eventos), atributos
  // aria-*, role, title, tabindex e data-*; o resto vira propriedade.
  function el(tag, props, ...children) {
    const n = document.createElement(tag);
    for (const [k, v] of Object.entries(props || {})) {
      if (v === undefined || v === null || v === false) continue;
      if (k === "class") n.className = v;
      else if (k === "text") n.textContent = v;
      else if (k === "on") for (const [ev, fn] of Object.entries(v)) n.addEventListener(ev, fn);
      else if (k === "role" || k === "title" || k === "tabindex" || k.startsWith("aria-") || k.startsWith("data-")) n.setAttribute(k, v === true ? "true" : String(v));
      else n[k] = v;
    }
    for (const c of children) {
      if (c === null || c === undefined || c === false) continue;
      n.append(typeof c === "string" ? document.createTextNode(c) : c);
    }
    return n;
  }

  /* ---------- API e sessão ---------- */

  const TOKEN_KEY = "ray-serve-token";
  let token = "";

  class ApiError extends Error {
    constructor(status, code, erro, detalhe) {
      super(detalhe ? erro + ": " + detalhe : erro);
      this.status = status;
      this.code = code;
      this.erro = erro;
      this.detalhe = detalhe;
    }
  }

  // api faz um pedido à própria origem com o token no cabeçalho; uma escrita
  // leva JSON. Uma resposta não-2xx vira ApiError com o código da API.
  async function api(method, path, body) {
    const headers = { "X-Ray-Token": token };
    const init = { method: method, headers: headers, cache: "no-store", credentials: "same-origin" };
    if (body !== undefined) {
      headers["Content-Type"] = "application/json";
      init.body = JSON.stringify(body);
    }
    let res;
    try {
      res = await fetch(path, init);
    } catch (e) {
      throw new ApiError(0, "network", "sem conexão com o ray serve", "o processo terminou?");
    }
    let data = null;
    try {
      data = await res.json();
    } catch (_) {
      data = null;
    }
    if (!res.ok) {
      const d = data || {};
      throw new ApiError(res.status, d.codigo || "http-" + res.status, d.erro || res.statusText || "erro", d.detalhe || "");
    }
    return data;
  }

  // readToken tira o token do fragmento (#token=<hex>), guarda-o na
  // sessionStorage e apaga o fragmento da URL e do histórico.
  function readToken() {
    let fromHash = "";
    if (location.hash) {
      fromHash = new URLSearchParams(location.hash.slice(1)).get("token") || "";
      history.replaceState(null, "", location.pathname + location.search);
    }
    try {
      if (fromHash) sessionStorage.setItem(TOKEN_KEY, fromHash);
      return fromHash || sessionStorage.getItem(TOKEN_KEY) || "";
    } catch (_) {
      return fromHash;
    }
  }

  // describe traduz um erro da API na frase da barra de status.
  function describe(err) {
    switch (err && err.code) {
      case "stale":
        return "o arquivo mudou no disco — recarregar";
      case "render-timeout":
        return "o template demora demais para renderizar; corrija antes de salvar";
      case "unauthorized":
        return "sessão inválida — reabra pelo ray serve";
      case undefined:
        return String(err);
      default:
        return err.message;
    }
  }

  function report(err) {
    setMessage(describe(err), "err");
  }

  /* ---------- Estado ---------- */

  const ui = {
    list: [], // GET /api/templates
    tabs: [], // abas abertas, em ordem
    active: null, // nome da aba ativa
    collapsed: new Set(), // pastas fechadas na árvore
    opening: new Set(),
    cursor: { line: 1, col: 1 },
    message: { text: "", kind: "" },
    messageTimer: 0,
    panel: "problems",
    resetArmed: 0, // nº de linhas do segundo passo do Restaurar (0 = desarmado)
    resetTimer: 0,
  };

  const tabOf = (name) => ui.tabs.find((t) => t.name === name) || null;
  const activeTab = () => (ui.active ? tabOf(ui.active) : null);
  const stackSample = () => $("sample").value === "true";
  const editable = (t) => !!t && !t.readOnly && t.utf8;

  /* ---------- Barra de status ---------- */

  function setMessage(text, kind) {
    ui.message = { text: text, kind: kind || "" };
    clearTimeout(ui.messageTimer);
    if (kind === "ok") ui.messageTimer = setTimeout(() => setMessage("", ""), 6000);
    renderStatus();
  }

  function renderStatus() {
    const t = activeTab();
    const bar = $("status");
    bar.classList.toggle("error", !!(t && t.render));

    const st = $("st-state");
    st.className = "st-item" + (t ? " state-" + t.state : "");
    st.textContent = t ? "● " + t.state + (t.readOnly ? " · somente leitura" : "") : "";

    const r = $("st-render");
    if (!t) r.textContent = "";
    else if (t.render) r.textContent = "✗ " + (t.render.line ? "linha " + t.render.line + ": " : "") + t.render.message;
    else if (t.previewKey) r.textContent = "✓ renderiza";
    else r.textContent = "… renderizando";

    const m = $("st-msg");
    m.className = "st-item st-msg" + (ui.message.kind ? " msg-" + ui.message.kind : "");
    m.textContent = ui.message.text ? (ui.message.kind === "err" ? "⚠ " : "") + ui.message.text : "";

    $("st-pos").textContent = t ? "Ln " + ui.cursor.line + ", Col " + ui.cursor.col : "";
    $("st-eol").textContent = t ? (t.eol === "crlf" ? "CRLF" : "LF") : "";
  }

  /* ---------- Árvore ---------- */

  async function refreshTree() {
    try {
      ui.list = await api("GET", "/api/templates");
    } catch (e) {
      report(e);
      return;
    }
    for (const t of ui.tabs) {
      const e = ui.list.find((x) => x.name === t.name);
      if (e) t.state = e.state;
    }
    renderTree();
    renderStatus();
  }

  function renderTree() {
    $("summary").textContent = ui.list.length ? summarize(ui.list) : "nenhum template";
    const tree = $("tree");
    const focusedName = document.activeElement && document.activeElement.dataset ? document.activeElement.dataset.key : "";
    const nodes = renderTreeLevel(buildTree(ui.list), 1);
    tree.replaceChildren(...nodes);
    const items = visibleItems();
    const keep = items.find((i) => i.dataset.key === focusedName);
    const current = items.find((i) => i.getAttribute("aria-selected") === "true");
    const roving = keep || current || items[0];
    if (roving) roving.tabIndex = 0;
    if (keep) keep.focus();
  }

  function renderTreeLevel(node, level) {
    return node.children.map((c) => {
      if (c.folder !== undefined) {
        const open = !ui.collapsed.has(c.path);
        const group = el("ul", { role: "group", class: "tree-group", hidden: !open }, ...renderTreeLevel(c, level + 1));
        return el(
          "li",
          { role: "treeitem", class: "tree-item folder", tabindex: -1, "aria-expanded": open ? "true" : "false", "aria-level": level, "data-key": "d:" + c.path, "data-folder": c.path },
          el("div", { class: "row", "data-level": Math.min(level, 6) }, el("span", { class: "twisty", text: open ? "▾" : "▸" }), el("span", { class: "label", text: c.folder })),
          group
        );
      }
      const e = c.entry;
      const t = tabOf(e.name);
      const title = e.name + " — " + e.state + (e.noBaseline ? " (sem linha-base)" : "") + (e.readOnly ? " — somente leitura na web" : "");
      return el(
        "li",
        { role: "treeitem", class: "tree-item file", tabindex: -1, "aria-level": level, "aria-selected": ui.active === e.name ? "true" : "false", "data-key": "f:" + e.name, "data-name": e.name, title: title },
        el(
          "div",
          { class: "row", "data-level": Math.min(level, 6) },
          el("span", { class: "dot state-" + e.state, "aria-hidden": "true", text: "●" }),
          el("span", { class: "label state-" + e.state, text: c.label }),
          t && t.dirty ? el("span", { class: "tree-dirty", text: "●", title: "rascunho não salvo" }) : null,
          e.readOnly ? el("span", { class: "ro", text: "somente leitura" }) : null,
          el("span", { class: "sr-only", text: ", " + e.state })
        )
      );
    });
  }

  function visibleItems() {
    return Array.from($("tree").querySelectorAll(".tree-item")).filter((li) => !li.closest(".tree-group[hidden]"));
  }

  function activateTreeItem(li) {
    if (li.dataset.folder !== undefined) {
      const p = li.dataset.folder;
      if (ui.collapsed.has(p)) ui.collapsed.delete(p);
      else ui.collapsed.add(p);
      renderTree();
      focusTreeKey(li.dataset.key);
    } else {
      openTemplate(li.dataset.name);
    }
  }

  function focusTreeKey(key) {
    const items = visibleItems();
    items.forEach((i) => (i.tabIndex = -1));
    const it = items.find((i) => i.dataset.key === key);
    if (it) {
      it.tabIndex = 0;
      it.focus();
    }
  }

  function wireTree() {
    const tree = $("tree");
    tree.addEventListener("click", (ev) => {
      const li = ev.target.closest(".tree-item");
      if (!li) return;
      focusTreeKey(li.dataset.key);
      activateTreeItem(li);
    });
    tree.addEventListener("keydown", (ev) => {
      const li = ev.target.closest(".tree-item");
      if (!li) return;
      const items = visibleItems();
      const i = items.indexOf(li);
      const isFolder = li.dataset.folder !== undefined;
      const expanded = li.getAttribute("aria-expanded") === "true";
      let target = null;
      switch (ev.key) {
        case "ArrowDown": target = items[i + 1]; break;
        case "ArrowUp": target = items[i - 1]; break;
        case "Home": target = items[0]; break;
        case "End": target = items[items.length - 1]; break;
        case "ArrowRight":
          if (isFolder && !expanded) activateTreeItem(li);
          else if (isFolder) target = items[i + 1];
          break;
        case "ArrowLeft":
          if (isFolder && expanded) activateTreeItem(li);
          else target = li.parentElement.closest(".tree-item");
          break;
        case "Enter":
        case " ":
          activateTreeItem(li);
          break;
        default:
          return;
      }
      ev.preventDefault();
      if (target) focusTreeKey(target.dataset.key);
    });
  }

  /* ---------- Abas ---------- */

  function renderTabs() {
    const bar = $("tabs");
    bar.replaceChildren(
      ...ui.tabs.map((t) => {
        const selected = t.name === ui.active;
        const base = t.name.split("/").pop();
        return el(
          "div",
          {
            role: "tab",
            class: "tab" + (selected ? " selected" : ""),
            "aria-selected": selected ? "true" : "false",
            tabindex: selected ? 0 : -1,
            title: t.name,
            "data-name": t.name,
          },
          t.render ? el("span", { class: "tab-err", text: "●", title: "não renderiza" }) : null,
          el("span", { class: "tab-label state-" + t.state, text: base }),
          t.dirty ? el("span", { class: "tab-dirty", text: "●", title: "rascunho não salvo" }) : null,
          el("button", { type: "button", class: "tab-close", text: "×", title: "Fechar", "aria-label": "Fechar " + t.name, tabindex: -1, "data-close": t.name })
        );
      })
    );
  }

  function wireTabs() {
    const bar = $("tabs");
    bar.addEventListener("click", (ev) => {
      const close = ev.target.closest(".tab-close");
      if (close) {
        ev.stopPropagation();
        closeTab(close.dataset.close);
        return;
      }
      const tab = ev.target.closest(".tab");
      if (tab) activate(tab.dataset.name);
    });
    bar.addEventListener("auxclick", (ev) => {
      const tab = ev.target.closest(".tab");
      if (tab && ev.button === 1) closeTab(tab.dataset.name);
    });
    bar.addEventListener("keydown", (ev) => {
      const i = ui.tabs.findIndex((t) => t.name === ui.active);
      if (i < 0) return;
      let j = -1;
      if (ev.key === "ArrowRight") j = (i + 1) % ui.tabs.length;
      else if (ev.key === "ArrowLeft") j = (i - 1 + ui.tabs.length) % ui.tabs.length;
      else if (ev.key === "Delete") {
        closeTab(ui.active);
        ev.preventDefault();
        return;
      } else return;
      ev.preventDefault();
      activate(ui.tabs[j].name);
      const sel = bar.querySelector(".tab.selected");
      if (sel) sel.focus();
    });
  }

  async function openTemplate(name) {
    if (tabOf(name)) {
      activate(name);
      return;
    }
    if (ui.opening.has(name)) return;
    ui.opening.add(name);
    try {
      const info = await api("GET", "/api/template?name=" + encodeURIComponent(name));
      if (tabOf(name)) return;
      const t = newTab(info);
      ui.tabs.push(t);
      activate(name);
      loadDiff(t);
    } catch (e) {
      report(e);
    } finally {
      ui.opening.delete(name);
    }
  }

  function newTab(info) {
    const t = {
      name: info.name,
      render: null, // {line,col,message} do último render do rascunho
      lastGood: null, // a saída do último render bom
      previewKey: "", // o que o preview mostrado renderizou
      warnings: [],
      diff: null,
      view: null, // cursor e rolagem do editor
      notice: null, // {kind: "stale"} ou {kind: "draft", text}
    };
    applyInfo(t, info);
    return t;
  }

  // applyInfo põe na aba o que GET /api/template devolveu e descarta o rascunho.
  function applyInfo(t, info) {
    const content = String(info.content || "").replace(/\r\n/g, "\n");
    t.state = info.state;
    t.eol = info.eol;
    t.utf8 = info.utf8 !== false;
    t.hash = info.hash;
    t.readOnly = !!info.readOnly;
    t.saved = content;
    t.draft = content;
    t.dirty = false;
    t.previewKey = "";
  }

  function closeTab(name) {
    const t = tabOf(name);
    if (!t) return;
    if (t.dirty && !window.confirm("Descartar as alterações não salvas de " + name + "?")) return;
    const i = ui.tabs.indexOf(t);
    ui.tabs.splice(i, 1);
    if (ui.active === name) {
      ui.active = null;
      const next = ui.tabs[i] || ui.tabs[i - 1];
      if (next) {
        activate(next.name);
        return;
      }
    }
    renderAll();
  }

  // activate mostra a aba name: o rascunho dela vai para o editor.
  function activate(name) {
    const prev = activeTab();
    if (prev && prev.name !== name) prev.view = editor.view();
    const t = tabOf(name);
    if (!t) return;
    const switching = ui.active !== name;
    ui.active = name;
    disarmReset();
    if (switching || editor.text() !== t.draft) {
      editor.load(t.draft);
      if (t.view) editor.restore(t.view);
    }
    editor.setReadOnly(!editable(t));
    editor.markError(t.render ? t.render.line : 0);
    renderAll();
    if (t.previewKey !== previewKey(t)) schedulePreview(0);
  }

  /* ---------- Editor: textarea + numeração + realce por sobreposição ---------- */

  // createEditor monta o editor em host. A interface mínima — load, text,
  // markError, onChange, focus, gotoLine — é o que a página usa para editar
  // e é o contrato para trocá-lo por outro editor; view, restore,
  // setReadOnly e onCursor são acessórios.
  function createEditor(host) {
    const gutter = el("div", { class: "gutter", "aria-hidden": "true" });
    const hl = el("pre", { class: "hl", "aria-hidden": "true" });
    const ta = el("textarea", { class: "ta", autocomplete: "off", "aria-label": "Conteúdo do template" });
    ta.spellcheck = false;
    ta.setAttribute("wrap", "off");
    ta.setAttribute("autocapitalize", "off");
    host.replaceChildren(gutter, el("div", { class: "code" }, hl, ta));

    const changeCbs = [];
    const cursorCbs = [];
    let errorLine = 0;
    let lineCount = 0;
    let gutterError = 0;
    let frame = 0;

    function paint() {
      frame = 0;
      const lines = ta.value.split("\n");
      const frag = document.createDocumentFragment();
      let mode = "";
      lines.forEach((line, i) => {
        const r = highlightLine(line, mode);
        mode = r.mode;
        const span = el("span", { class: i + 1 === errorLine ? "ln err-line" : "ln" });
        for (const [text, cls] of r.tokens) span.append(cls ? el("span", { class: cls, text: text }) : document.createTextNode(text));
        if (!line && i + 1 === errorLine) span.append(document.createTextNode(" "));
        frag.append(span, document.createTextNode("\n"));
      });
      hl.replaceChildren(frag);
      if (lines.length !== lineCount || gutterError !== errorLine) {
        lineCount = lines.length;
        gutterError = errorLine;
        const nums = [];
        for (let i = 1; i <= lineCount; i++) nums.push(el("div", { class: i === errorLine ? "num err" : "num", text: String(i) }));
        gutter.replaceChildren(...nums);
      }
      sync();
    }

    function schedule() {
      if (!frame) frame = requestAnimationFrame(paint);
    }

    function sync() {
      hl.scrollTop = ta.scrollTop;
      hl.scrollLeft = ta.scrollLeft;
      gutter.scrollTop = ta.scrollTop;
    }

    function cursor() {
      const before = ta.value.slice(0, ta.selectionStart);
      const nl = before.lastIndexOf("\n");
      const pos = { line: before.split("\n").length, col: ta.selectionStart - nl };
      cursorCbs.forEach((cb) => cb(pos));
    }

    ta.addEventListener("input", () => {
      schedule();
      cursor();
      changeCbs.forEach((cb) => cb(ta.value));
    });
    ta.addEventListener("scroll", sync);
    ["keyup", "mouseup", "focus", "select"].forEach((ev) => ta.addEventListener(ev, cursor));

    return {
      load(text) {
        ta.value = text;
        ta.setSelectionRange(0, 0);
        ta.scrollTop = 0;
        ta.scrollLeft = 0;
        paint();
        cursor();
      },
      text() {
        return ta.value;
      },
      markError(line) {
        const n = line | 0;
        if (n === errorLine) return;
        errorLine = n;
        schedule();
      },
      onChange(cb) {
        changeCbs.push(cb);
      },
      focus() {
        ta.focus();
      },
      gotoLine(n, col) {
        const lines = ta.value.split("\n");
        const line = Math.max(1, Math.min(n | 0, lines.length));
        let at = 0;
        for (let i = 0; i < line - 1; i++) at += lines[i].length + 1;
        at += Math.max(0, Math.min((col | 0) - 1, lines[line - 1].length));
        ta.focus();
        ta.setSelectionRange(at, at);
        const lh = parseFloat(getComputedStyle(ta).lineHeight) || 19;
        ta.scrollTop = Math.max(0, (line - 1) * lh - ta.clientHeight / 2);
        sync();
        cursor();
      },
      view() {
        return { start: ta.selectionStart, end: ta.selectionEnd, top: ta.scrollTop, left: ta.scrollLeft };
      },
      restore(v) {
        ta.setSelectionRange(v.start, v.end);
        ta.scrollTop = v.top;
        ta.scrollLeft = v.left;
        sync();
        cursor();
      },
      setReadOnly(ro) {
        ta.readOnly = !!ro;
        host.classList.toggle("readonly", !!ro);
      },
      onCursor(cb) {
        cursorCbs.push(cb);
      },
    };
  }

  let editor = null;

  function onEdit(text) {
    const t = activeTab();
    if (!t) return;
    const wasDirty = t.dirty;
    t.draft = text;
    t.dirty = text !== t.saved;
    if (wasDirty !== t.dirty) {
      renderTabs();
      renderTree();
    }
    schedulePreview(400);
  }

  /* ---------- Preview: 400 ms depois da última tecla, um pedido por vez ---------- */

  const preview = { timer: 0, inflight: false, again: false };

  const previewKey = (t) => (stackSample() ? "1:" : "0:") + t.draft;

  function schedulePreview(delay) {
    clearTimeout(preview.timer);
    preview.timer = setTimeout(runPreview, delay);
  }

  async function runPreview() {
    const t = activeTab();
    if (!t) return;
    if (preview.inflight) {
      preview.again = true;
      return;
    }
    const key = previewKey(t);
    if (t.previewKey === key) return;
    preview.inflight = true;
    let retry = false;
    try {
      const r = await api("POST", "/api/preview", { name: t.name, content: t.draft, stack: stackSample() });
      if (previewKey(t) === key) applyPreview(t, key, r);
    } catch (e) {
      if (e.code === "busy") retry = true;
      else report(e);
    } finally {
      preview.inflight = false;
    }
    if (retry) schedulePreview(600);
    else if (preview.again) {
      preview.again = false;
      schedulePreview(0);
    }
  }

  function applyPreview(t, key, r) {
    t.previewKey = key;
    t.render = r.err || null;
    if (!r.err) t.lastGood = r.output || "";
    if (t.name !== ui.active) {
      renderTabs();
      return;
    }
    editor.markError(t.render ? t.render.line : 0);
    renderTabs();
    renderPreview();
    renderProblems();
    renderStatus();
  }

  function renderPreview() {
    const t = activeTab();
    const pre = $("preview");
    if (!t) {
      pre.textContent = "";
      return;
    }
    const stale = !!t.render;
    pre.classList.toggle("stale", stale);
    $("preview-flag").hidden = !stale || t.lastGood === null;
    if (t.lastGood === null) {
      pre.textContent = t.render ? "(o rascunho ainda não renderizou)" : "renderizando…";
      pre.classList.add("placeholder");
    } else {
      pre.classList.remove("placeholder");
      pre.textContent = t.lastGood;
    }
  }

  /* ---------- Painel: Problemas e Diff ---------- */

  function renderProblems() {
    const t = activeTab();
    const box = $("problems");
    const rows = [];
    if (t && t.render) {
      const r = t.render;
      const where = r.line ? "linha " + r.line + (r.col ? ", col " + r.col : "") + ": " : "";
      rows.push(
        el(
          "button",
          { type: "button", class: "problem err", disabled: !r.line, on: { click: () => editor.gotoLine(r.line, r.col) } },
          el("span", { class: "problem-icon", text: "✗" }),
          el("span", { text: where + r.message })
        )
      );
    }
    for (const w of (t && t.warnings) || []) {
      rows.push(el("div", { class: "problem warn" }, el("span", { class: "problem-icon", text: "⚠" }), el("span", { text: w })));
    }
    if (!rows.length) rows.push(el("p", { class: "muted", text: t ? "nenhum problema" : "" }));
    box.replaceChildren(...rows);
    const badge = $("problem-count");
    const n = (t && t.render ? 1 : 0) + ((t && t.warnings.length) || 0);
    badge.hidden = n === 0;
    badge.textContent = String(n);
  }

  async function loadDiff(t) {
    try {
      t.diff = await api("GET", "/api/diff?name=" + encodeURIComponent(t.name));
    } catch (e) {
      t.diff = { error: describe(e) };
    }
    if (t.name === ui.active) renderDiff();
  }

  const DIFF_KIND = {
    current: "= idêntico ao padrão embutido",
    new: "+ ainda não foi criado: o próximo init ai escreve o padrão embutido",
    custom: "template próprio: não há padrão embutido para comparar",
    "crlf-only": "~ só o fim de linha (CRLF) difere do padrão embutido",
    same: "= nenhuma diferença de conteúdo",
  };

  function renderDiff() {
    const t = activeTab();
    const box = $("diff");
    const d = t && t.diff;
    if (!d) {
      box.replaceChildren(el("p", { class: "muted", text: t ? "carregando…" : "" }));
      return;
    }
    if (d.error) {
      box.replaceChildren(el("p", { class: "problem warn", text: "⚠ " + d.error }));
      return;
    }
    const parts = [el("p", { class: "diff-head muted", text: "salvo × padrão embutido" })];
    if (d.outdated) {
      parts.push(el("p", { class: "diff-note state-outdated", text: "⚠ outdated: o padrão embutido mudou e você não tocou neste arquivo — Restaurar padrão traz a versão nova" }));
    }
    if (d.kind === "unified" && d.text) {
      const pre = el("pre", { class: "diff-text" });
      for (const line of d.text.replace(/\n$/, "").split("\n")) {
        pre.append(el("span", { class: "d-line " + diffLineClass(line), text: line }), document.createTextNode("\n"));
      }
      parts.push(pre);
    } else {
      parts.push(el("p", { class: "diff-note", text: DIFF_KIND[d.kind] || d.kind }));
    }
    box.replaceChildren(...parts);
  }

  function selectPanel(which) {
    ui.panel = which;
    for (const [tabId, bodyId] of [["ptab-problems", "problems"], ["ptab-diff", "diff"]]) {
      const on = bodyId === which;
      $(tabId).setAttribute("aria-selected", on ? "true" : "false");
      $(tabId).tabIndex = on ? 0 : -1;
      $(bodyId).hidden = !on;
    }
  }

  function wirePanel() {
    $("ptab-problems").addEventListener("click", () => selectPanel("problems"));
    $("ptab-diff").addEventListener("click", () => selectPanel("diff"));
    for (const id of ["ptab-problems", "ptab-diff"]) {
      $(id).addEventListener("keydown", (ev) => {
        if (ev.key !== "ArrowLeft" && ev.key !== "ArrowRight") return;
        ev.preventDefault();
        const next = ui.panel === "problems" ? "diff" : "problems";
        selectPanel(next);
        $(next === "problems" ? "ptab-problems" : "ptab-diff").focus();
      });
    }
  }

  /* ---------- Ações: salvar, restaurar, recarregar ---------- */

  let saving = false;

  async function save() {
    const t = activeTab();
    if (!editable(t) || saving) return;
    saving = true;
    const draft = t.draft;
    setMessage("salvando…", "");
    try {
      const r = await api("PUT", "/api/template", { name: t.name, content: restoreEOL(draft, t.eol), baseHash: t.hash });
      t.hash = r.hash;
      t.state = r.state;
      t.saved = draft;
      t.dirty = t.draft !== draft;
      t.notice = null;
      t.warnings = (r.warnings || []).slice();
      if (r.baselineErr) t.warnings.push("linha-base: " + r.baselineErr);
      if (r.render) {
        t.render = r.render;
        t.previewKey = "";
        setMessage("salvo, mas não renderiza: " + (r.render.line ? "linha " + r.render.line + ": " : "") + r.render.message, "err");
      } else {
        setMessage(r.outcome === "no changes" ? "salvo — nada mudou" : "salvo · " + r.state, "ok");
      }
      if (t.name === ui.active) editor.markError(t.render ? t.render.line : 0);
      renderAll();
      refreshTree();
      loadDiff(t);
    } catch (e) {
      if (e.code === "stale") {
        t.notice = { kind: "stale" };
        renderNotice();
      }
      report(e);
    } finally {
      saving = false;
    }
  }

  // reload relê o template do disco. keepDraft guarda o rascunho anterior
  // num aviso, para o usuário copiar o que precisar.
  async function reload(t, keepDraft) {
    let info;
    try {
      info = await api("GET", "/api/template?name=" + encodeURIComponent(t.name));
    } catch (e) {
      report(e);
      return false;
    }
    const old = t.draft;
    applyInfo(t, info);
    t.render = null;
    t.notice = keepDraft && old !== t.saved ? { kind: "draft", text: old } : null;
    t.view = null;
    if (t.name === ui.active) {
      editor.load(t.draft);
      editor.setReadOnly(!editable(t));
      editor.markError(0);
    }
    renderAll();
    schedulePreview(0);
    loadDiff(t);
    refreshTree();
    return true;
  }

  async function reset(force) {
    const t = activeTab();
    if (!editable(t)) return;
    if (!force && t.dirty && !window.confirm("O rascunho não salvo de " + t.name + " será descartado. Restaurar o padrão?")) return;
    disarmReset();
    try {
      const r = await api("POST", "/api/reset", { name: t.name, force: !!force });
      const said = { restored: "restaurado", created: "restaurado (criado a partir do padrão)" }[r.action] || "já é o padrão";
      if (await reload(t, false)) {
        if (r.baselineErr) t.warnings = ["linha-base: " + r.baselineErr];
        else t.warnings = [];
        renderProblems();
        setMessage(said, r.baselineErr ? "err" : "ok");
      }
    } catch (e) {
      if (e.code === "edited") armReset(editedLines(e.detalhe));
      else report(e);
    }
  }

  // O segundo passo do Restaurar: o botão vira a pergunta e volta sozinho.
  function armReset(lines) {
    ui.resetArmed = lines || -1;
    const b = $("reset");
    b.textContent = lines ? "descartar as suas edições (" + lines + (lines === 1 ? " linha" : " linhas") + ")?" : "descartar as suas edições?";
    b.classList.add("danger");
    $("reset-cancel").hidden = false;
    clearTimeout(ui.resetTimer);
    ui.resetTimer = setTimeout(disarmReset, 10000);
    setMessage("o template tem edições suas: confirme para descartá-las", "");
    b.focus();
  }

  function disarmReset() {
    ui.resetArmed = 0;
    clearTimeout(ui.resetTimer);
    const b = $("reset");
    b.textContent = "Restaurar padrão";
    b.classList.remove("danger");
    $("reset-cancel").hidden = true;
  }

  function renderNotice() {
    const t = activeTab();
    const box = $("notice");
    const parts = [];
    if (t && t.readOnly) {
      parts.push(el("p", {}, "hook: somente leitura na web, edite com ", el("code", { text: "ray template edit " + t.name })));
    } else if (t && !t.utf8) {
      parts.push(el("p", {}, "o arquivo não é UTF-8: somente leitura na web, edite com ", el("code", { text: "ray template edit " + t.name })));
    }
    if (t && t.notice && t.notice.kind === "stale") {
      parts.push(
        el(
          "p",
          { class: "notice-warn" },
          "⚠ o arquivo mudou no disco depois que você o abriu. ",
          el("button", { type: "button", class: "btn", text: "recarregar", on: { click: () => reload(t, true) } }),
          " (o seu rascunho fica guardado aqui)"
        )
      );
    }
    if (t && t.notice && t.notice.kind === "draft") {
      const text = t.notice.text;
      parts.push(
        el(
          "details",
          { class: "draft" },
          el("summary", { text: "recarregado do disco — o seu rascunho anterior" }),
          el("pre", { class: "draft-text", text: text }),
          el(
            "p",
            {},
            el("button", {
              type: "button",
              class: "btn",
              text: "copiar rascunho",
              on: { click: () => copyText(text) },
            }),
            " ",
            el("button", {
              type: "button",
              class: "btn",
              text: "descartar",
              on: {
                click: () => {
                  t.notice = null;
                  renderNotice();
                },
              },
            })
          )
        )
      );
    }
    box.replaceChildren(...parts);
    box.hidden = parts.length === 0;
  }

  async function copyText(text) {
    try {
      await navigator.clipboard.writeText(text);
      setMessage("rascunho copiado", "ok");
    } catch (_) {
      setMessage("não foi possível copiar; selecione o texto do rascunho", "err");
    }
  }

  /* ---------- Montagem ---------- */

  function renderToolbar() {
    const t = activeTab();
    $("crumb").textContent = t ? t.name : "";
    $("crumb").className = "crumb" + (t ? " state-" + t.state : "");
    $("save").disabled = !editable(t);
    $("reset").disabled = !editable(t) || t.state === "custom";
    $("reset").title = t && t.state === "custom" ? "template próprio: não há padrão para restaurar" : "Restaurar o padrão embutido";
  }

  function renderAll() {
    const t = activeTab();
    $("workspace").hidden = !t;
    $("empty").hidden = !!t;
    renderTabs();
    renderTree();
    renderToolbar();
    renderNotice();
    renderPreview();
    renderProblems();
    renderDiff();
    renderStatus();
  }

  function wire() {
    editor = createEditor($("editor"));
    editor.onChange(onEdit);
    editor.onCursor((pos) => {
      ui.cursor = pos;
      renderStatus();
    });
    wireTree();
    wireTabs();
    wirePanel();
    $("refresh").addEventListener("click", refreshTree);
    $("save").addEventListener("click", save);
    $("reset").addEventListener("click", () => reset(ui.resetArmed !== 0));
    $("reset-cancel").addEventListener("click", () => {
      disarmReset();
      setMessage("", "");
    });
    $("sample").addEventListener("change", () => schedulePreview(0));
    document.addEventListener("keydown", (ev) => {
      if ((ev.ctrlKey || ev.metaKey) && !ev.altKey && ev.key.toLowerCase() === "s") {
        ev.preventDefault();
        save();
      }
    });
    window.addEventListener("beforeunload", (ev) => {
      if (ui.tabs.some((t) => t.dirty)) {
        ev.preventDefault();
        ev.returnValue = "";
      }
    });
    window.addEventListener("unhandledrejection", (ev) => {
      ev.preventDefault();
      report(ev.reason);
    });
  }

  function boot() {
    token = readToken();
    if (!token) {
      $("app").hidden = true;
      $("gate").hidden = false;
      return;
    }
    wire();
    renderAll();
    refreshTree();
  }

  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", boot);
  else boot();
})();
