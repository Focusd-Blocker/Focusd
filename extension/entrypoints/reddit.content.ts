import "../assets/reddit.css";

export default defineContentScript({
  matches: ["*://*.reddit.com/*", "*://reddit.com/*"],
  runAt: "document_start",

  main() {
    let hasReportedThisSession = false;

    const nsfwSelectors = [
      "shreddit-post[nsfw]",
      'shreddit-blurred-container[reason="nsfw"]',
      ".thing.over18",
      '[data-testid="search-post-unit"]:has(.text-category-nsfw)',
      '[data-testid="search-post-unit"]:has(svg[icon-name*="nsfw"])',
      '[data-id="search-media-post-unit"]:has(.text-category-nsfw)',
      '[data-id="search-media-post-unit"]:has(svg[icon-name*="nsfw"])',
      '[data-testid="search-community-unit"]:has(.text-category-nsfw)',
      '[data-testid="search-community-unit"]:has(svg[icon-name*="nsfw"])',
      '[data-testid="search-user-unit"]:has(.text-category-nsfw)',
      '[data-testid="search-user-unit"]:has(svg[icon-name*="nsfw"])',
      'div:has(> faceplate-hovercard):has(> [data-testid="search-warnings"] .text-category-nsfw)',
      'div:has(> faceplate-hovercard):has(> [data-testid="search-warnings"] svg[icon-name*="nsfw"])',
    ].join(", ");

    function nukeNSFWPosts() {
      const rawElements = Array.from(document.querySelectorAll(nsfwSelectors));

      const nsfwElements = rawElements.filter((el) => {
        const parent = el.parentElement;
        return parent ? !parent.closest(nsfwSelectors) : true;
      });

      if (nsfwElements.length > 0) {
        if (!hasReportedThisSession) {
          browser.runtime.sendMessage({
            action: "report_event",
            domain: "reddit.com",
            source: "reddit_nsfw_filter",
            count: nsfwElements.length,
          });
          hasReportedThisSession = true;
          setTimeout(() => {
            hasReportedThisSession = false;
          }, 60000);
        }
      }

      nsfwElements.forEach((element) => {
        if (element.getAttribute("data-nsfw-nuked") === "true") return;
        element.setAttribute("data-nsfw-nuked", "true");

        const warningBox = document.createElement("div");
        warningBox.innerHTML = `
          <div style="
            background: #0d0d0d;
            border: 2px solid #ff4757;
            color: #ff4757;
            padding: 15px;
            text-align: center;
            font-family: sans-serif;
            margin: 5px;
            border-radius: 8px;
            width: 100%;
            height: 100%;
            min-height: 100px;
            display: flex;
            flex-direction: column;
            justify-content: center;
            box-sizing: border-box;
          ">
            <h3 style="margin: 0 0 5px 0; font-size: 1.1rem; letter-spacing: 1px;">BLOCKED</h3>
            <p style="margin: 0; color: #a4b0be; font-size: 0.85rem;">This post type is not allowed, keep scrolling.</p>
          </div>
        `;

        if (element.parentNode) {
          element.parentNode.insertBefore(warningBox, element);
        }
      });
    }

    const observer = new MutationObserver(() => {
      nukeNSFWPosts();
    });

    observer.observe(document.documentElement, {
      childList: true,
      subtree: true,
    });
  },
});
