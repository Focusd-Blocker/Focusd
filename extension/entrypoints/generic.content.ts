export default defineContentScript({
  matches: ["<all_urls>"],

  excludeMatches: [
    "*://*.reddit.com/*",
    "*://reddit.com/*",
    "*://*.x.com/*",
    "*://*.twitter.com/*",
    "*://*.google.com/*",
    "*://*.duckduckgo.com/*",
    "*://*.bing.com/*"
  ],
  runAt: "document_idle",

  main() {
    if (sessionStorage.getItem("focusd-override") === "true") return;

    let isSuspect = false;

    const metaRating = document.querySelector('meta[name="rating" i]')?.getAttribute("content")?.toLowerCase() || "";
    if (metaRating.includes("adult") || metaRating.includes("rta-5042")) {
      isSuspect = true;
    }

    if (!isSuspect) {
      const pageText = document.body.textContent?.toLowerCase() || "";

      const badWords = [
        "porn", "pornography", "nsfw", "xxx", "smut", "erotica",

        "pornhub", "xvideos", "xhamster", "redtube", "youporn", "chaturbate",
        "onlyfans", "fansly", "brazzers", "spankbang", "eporner",

        "hentai", "yiff", "rule34", "rule 34", "ecchi", "ahegao", "doujinshi", "eroge",

        "blowjob", "handjob", "cumshot", "creampie", "gangbang", "threesome",
        "milf", "bukkake", "orgasm", "ejaculation", "masturbation",

        "bdsm", "bondage", "sadomasochism", "dominatrix",
        "sex toy", "dildo", "vibrator", "fleshlight", "buttplug",
        "camgirl", "cam model",

        "escort", "prostitute", "brothel", "hooker", "sugar daddy"
      ];

      let dangerScore = 0;

      for (const word of badWords) {
        const regex = new RegExp(`\\b${word}\\b`, "g");
        const matches = pageText.match(regex);
        if (matches) {
          dangerScore += matches.length;
        }
      }

      if (dangerScore >= 4) {
        isSuspect = true;
      }
    }

    if (isSuspect) {
      browser.runtime.sendMessage({
        action: "report_event",
        domain: window.location.hostname,
        source: "generic_heuristic_net",
        count: 1,
      });

      document.body.style.overflow = "hidden";

      const overlay = document.createElement("div");
      overlay.id = "focusd-generic-overlay";
      overlay.style.cssText = `
        position: fixed;
        inset: 0;
        z-index: 2147483647; /* Maximum possible z-index */
        background: rgba(20, 24, 26, 0.85);
        backdrop-filter: blur(16px);
        -webkit-backdrop-filter: blur(16px);
        display: flex;
        flex-direction: column;
        align-items: center;
        justify-content: center;
        font-family: system-ui, sans-serif;
        color: #eae6dd;
        text-align: center;
        padding: 24px;
      `;

      overlay.innerHTML = `
        <div style="max-width: 420px; background: #1a1e21; padding: 32px; border-radius: 12px; border: 1px solid #2a3033; box-shadow: 0 10px 30px rgba(0,0,0,0.5);">
          <h2 style="margin: 0 0 12px 0; color: #ff4757; font-size: 1.5rem; letter-spacing: 1px;">SENSITIVE CONTENT DETECTED</h2>
          <p style="margin: 0 0 32px 0; color: #838d90; font-size: 1rem; line-height: 1.5;">
            Focusd blocked this page because it contains a high amount of adult keywords or meta tags.
          </p>

          <div style="display: flex; flex-direction: column; gap: 12px; width: 100%;">
            <button id="focusd-leave-btn" style="
              background: #c98a4b; color: #1c1712; border: none; padding: 14px;
              font-size: 1rem; font-weight: 600; border-radius: 6px; cursor: pointer; transition: 0.15s;
            ">Get me out of here</button>

            <button id="focusd-bypass-btn" disabled style="
              background: transparent; color: #838d90; border: 1px solid #2a3033; padding: 10px;
              font-size: 0.85rem; border-radius: 6px; cursor: not-allowed; transition: 0.15s; opacity: 0.5;
            ">This is a mistake, let me in (wait 5s)</button>
          </div>
        </div>
      `;

      document.body.appendChild(overlay);

      document.getElementById("focusd-leave-btn")?.addEventListener("click", () => {
        browser.runtime.sendMessage({ action: "close_current_tab" });
      });

      const bypassBtn = document.getElementById("focusd-bypass-btn") as HTMLButtonElement;
      if (bypassBtn) {
        let timeLeft = 5;
        const interval = setInterval(() => {
          timeLeft--;
          if (timeLeft <= 0) {
            clearInterval(interval);
            bypassBtn.disabled = false;
            bypassBtn.innerText = "This is a mistake, let me in";
            bypassBtn.style.cursor = "pointer";
            bypassBtn.style.opacity = "1";
          } else {
            bypassBtn.innerText = `This is a mistake, let me in (wait ${timeLeft}s)`;
          }
        }, 1000);

        bypassBtn.addEventListener("click", () => {
          sessionStorage.setItem("focusd-override", "true");
          overlay.remove();
          document.body.style.overflow = "";
        });
      }
    }
  },
});
