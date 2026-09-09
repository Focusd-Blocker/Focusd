export default defineBackground(() => {
  let blockedDomains = new Set<string>();
  const DAEMON_URL = "http://127.0.0.1:36287";

  browser.storage.local.get("blocklist").then((res) => {
    if (Array.isArray(res.blocklist)) {
      blockedDomains = new Set(
        res.blocklist.filter((domain): domain is string => typeof domain === "string"),
      );
    }
  });

  async function refreshBlocklist() {
    try {
      const res = await fetch(`${DAEMON_URL}/blocklist`);
      if (!res.ok) throw new Error(`status ${res.status}`);
      const data = await res.json();
      const domains = Array.isArray(data.domains)
        ? data.domains.filter(
            (domain: unknown): domain is string => typeof domain === "string",
          )
        : [];
      blockedDomains = new Set(domains);
      await browser.storage.local.set({ blocklist: domains });
    } catch (err) {
      console.error("focusd: failed to refresh blocklist", err);
    }
  }

  refreshBlocklist();

  try {
    const es = new EventSource(`${DAEMON_URL}/stream`);
    es.addEventListener("update", () => {
      console.log("focusd: received live blocklist update signal");
      refreshBlocklist();
    });
  } catch (err) {
    console.error("focusd: failed to connect to daemon stream", err);
  }

  async function reportEvent(
    domain: string,
    source: string,
    count: number = 1,
  ) {
    if (!domain || !source || !Number.isFinite(count) || count <= 0) return;
    try {
      await fetch(`${DAEMON_URL}/event`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ domain, source, count }),
      });
    } catch (err) {
      console.error("focusd: failed to report event", err);
    }
  }

  async function sendHeartbeat() {
    try {
      await fetch(`${DAEMON_URL}/heartbeat`, { method: "POST" });
    } catch {}
  }


  function isDomainOrSubdomainBlocked(targetHostname: string, domains: Set<string>): boolean {
    let host = targetHostname.toLowerCase().replace(/\.$/, "").replace(/^www\./, "");
    while (host) {
      if (domains.has(host)) return true
      const dotIndex = host.indexOf('.');
      if (dotIndex === -1) break;
      host = host.slice(dotIndex + 1);
    }
    return false
  }

  function extractEmbeddedHostnames(url: string): string[] {
    const hosts: string[] = [];
    try {
      const parsed = new URL(url);
      for (const [key, value] of parsed.searchParams.entries()) {
        try {
          const embeddedParsed = new URL(value);
          hosts.push(embeddedParsed.hostname.toLowerCase().replace(/^www\./, ""));
        } catch {}
      }
    } catch {}

    const match = url.match(/\/(https?)\/([^/?#]+)/i);
    if (match && match[2]) {
      hosts.push(match[2].toLowerCase().replace(/^www\./, ""));
    }
    return hosts;
  }

  function isSearchEngineProxy(hostname: string): boolean {
    const host = hostname.toLowerCase().replace(/\.$/, "");
    return (
      host === "duckduckgo.com" ||
      host.endsWith(".duckduckgo.com") ||
      host === "bing.com" ||
      host.endsWith(".bing.com") ||
      host === "google.com" ||
      host.endsWith(".google.com") ||
      host === "yahoo.com" ||
      host.endsWith(".yahoo.com")
    );
  }

  function getBlockedDomain(url: string, domains: Set<string>): string | null {
    try {
      const parsed = new URL(url);
      const hostname = parsed.hostname.toLowerCase().replace(/\.$/, "").replace(/^www\./, "");

      if (isDomainOrSubdomainBlocked(hostname, domains)) {
        return hostname;
      }

      // Search engines proxy image/results URLs through query parameters. The
      // embedded host is not the site the user is visiting, so do not treat it
      // as a navigated destination.
      if (isSearchEngineProxy(hostname)) {
        return null;
      }

      const embeddedHosts = extractEmbeddedHostnames(url);
      for (const embeddedHost of embeddedHosts) {
        if (isDomainOrSubdomainBlocked(embeddedHost, domains)) {
          return embeddedHost;
        }
      }
    } catch { }
    return null;
  }

  browser.alarms.create("refresh-blocklist", { periodInMinutes: 360 });
  browser.alarms.create("heartbeat-alarm", { periodInMinutes: 1 });

  browser.alarms.onAlarm.addListener((alarm) => {
    if (alarm.name === "refresh-blocklist") refreshBlocklist();
    if (alarm.name === "heartbeat-alarm") sendHeartbeat();
  });

  browser.runtime.onMessage.addListener(async (message, sender) => {
    if (message.action === "report_event") {
      reportEvent(message.domain, message.source, message.count);
    }

    if (message.action === "close_current_tab" && sender.tab?.id) {
      await browser.tabs.remove(sender.tab.id);
    }
  });

  browser.webRequest.onBeforeRequest.addListener(
    (details) => {
      const blockedDomain = getBlockedDomain(details.url, blockedDomains);
      if (blockedDomain) {
        reportEvent(blockedDomain, "web_request");

        if (details.type === "image") {
          return { redirectUrl: browser.runtime.getURL("/stop.png") };
        }

        if (details.type === "main_frame" || details.type === "sub_frame") {
          return { redirectUrl: browser.runtime.getURL("/blocked.html") };
        }

        return { cancel: true };
      }
    },
    { urls: ["<all_urls>"] },
    ["blocking"],
  );
});
