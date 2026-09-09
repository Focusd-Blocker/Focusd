import { defineConfig } from "wxt";
import packageJson from "./package.json";

export default defineConfig({
  manifestVersion: 2,
  manifest: {
    name: "Focusd",
    description: "Stay focused and in control.",
    version: packageJson.version,
    icons: {
      16: "icon.png",
      32: "icon.png",
      48: "icon.png",
      96: "icon.png",
      128: "icon.png",
    },

    permissions: [
      "webRequest",
      "webRequestBlocking",
      "<all_urls>",
      "storage",
      "alarms",
      "http://127.0.0.1/*",
    ],

    web_accessible_resources: ["blocked.html", "stop.png"],

    browser_specific_settings: {
      gecko: {
        id: "{5f1c1d4d-c0f0-41bc-862b-0c7f8b860beb}",
        data_collection_permissions: {
          required: [
            "browsingActivity"
          ]
        }
      },
    },
  },
  modules: ["@wxt-dev/module-vue"],
});
