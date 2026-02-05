(function () {
  let tocGlobalListenersBound = false;
  let tocSyncTimer = null;

  function normalizeHash(value) {
    if (!value) return "";
    try {
      return decodeURIComponent(value);
    } catch (_err) {
      return value;
    }
  }

  function getLinkHash(link) {
    if (!link) return "";
    try {
      return normalizeHash(new URL(link.href, window.location.href).hash);
    } catch (_err) {
      return normalizeHash(link.hash || "");
    }
  }

  function initMermaid() {
    if (!window.mermaid) return;

    mermaid.initialize({
      startOnLoad: false,
      theme: "base",
      themeVariables: {
        primaryColor: "#fff7ed",
        primaryTextColor: "#0f172a",
        primaryBorderColor: "#e2e8f0",
        lineColor: "#94a3b8",
        fontFamily: "IBM Plex Sans, system-ui, sans-serif",
        fontSize: "14px",
      },
    });

    const nodes = Array.from(document.querySelectorAll(".mermaid"));
    if (!nodes.length) return;
    const targets = nodes.filter((node) => !node.dataset.mermaidRendered);
    if (!targets.length) return;

    const done = () => {
      targets.forEach((node) => {
        node.dataset.mermaidRendered = "true";
      });
    };

    const result = mermaid.run({ nodes: targets });
    if (result && typeof result.then === "function") {
      result.then(done).catch(() => {});
    } else {
      done();
    }
  }

  function syncTOCHashActive(preferredHash) {
    const links = Array.from(
      document.querySelectorAll('.md-nav--secondary a.md-nav__link[href*="#"]')
    );
    const navs = Array.from(document.querySelectorAll(".md-nav--secondary"));
    if (!links.length) return;

    links.forEach((link) => link.classList.remove("r-toc-hash-active"));
    navs.forEach((nav) => nav.classList.remove("r-toc-hash-mode"));

    const hash = normalizeHash(preferredHash || window.location.hash || "");
    if (!hash || hash === "#") return;

    const targets = links.filter((link) => getLinkHash(link) === hash);

    if (!targets.length) return;

    targets.forEach((target) => {
      target.classList.add("r-toc-hash-active");
      const container = target.closest(".md-nav--secondary");
      if (container) container.classList.add("r-toc-hash-mode");
    });
  }

  function scheduleTOCSync(preferredHash) {
    if (tocSyncTimer) {
      window.clearTimeout(tocSyncTimer);
      tocSyncTimer = null;
    }
    window.requestAnimationFrame(() => syncTOCHashActive(preferredHash));
    window.setTimeout(() => syncTOCHashActive(preferredHash), 90);
    tocSyncTimer = window.setTimeout(() => {
      syncTOCHashActive(preferredHash);
      tocSyncTimer = null;
    }, 260);
  }

  function bindTOCClicks() {
    const links = document.querySelectorAll('.md-nav--secondary a.md-nav__link[href*="#"]');
    links.forEach((link) => {
      if (link.dataset.ranyaTocBound === "true") return;
      link.dataset.ranyaTocBound = "true";
      link.addEventListener("click", () => {
        const linkHash = getLinkHash(link);
        scheduleTOCSync(linkHash);
      });
    });
  }

  function bindTOCGlobalListeners() {
    if (tocGlobalListenersBound) return;
    tocGlobalListenersBound = true;
    window.addEventListener("hashchange", () => {
      scheduleTOCSync();
    });
  }

  function initPageEnhancements() {
    initMermaid();
    bindTOCGlobalListeners();
    bindTOCClicks();
    scheduleTOCSync();
  }

  if (window.document$ && window.document$.subscribe) {
    window.document$.subscribe(initPageEnhancements);
    return;
  }

  if (document.addEventListener) {
    document.addEventListener("DOMContentLoaded", initPageEnhancements);
  }
})();
