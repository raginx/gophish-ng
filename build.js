// build.js
//
// Builds the Gophish frontend assets: concatenates + minifies the vendor JS
// bundle, transpiles + minifies each app script individually (since
// different pages load different combinations of them), and concatenates +
// minifies the CSS bundle.
//
// Replaces the previous Gulp + Webpack setup, which independently processed
// some of the same app files (passwords.js, users.js, webhooks.js) into the
// same output path.

const esbuild = require("esbuild");
const fs = require("fs");
const path = require("path");

const jsSrcDir = path.join("static", "js", "src");
const vendorDir = path.join(jsSrcDir, "vendor");
const appDir = path.join(jsSrcDir, "app");
const cssDir = path.join("static", "css");
const jsDistDir = path.join("static", "js", "dist");
const cssDistDir = path.join("static", "css", "dist");

const vendorFiles = [
  "jquery.js",
  "bootstrap.min.js",
  "moment.min.js",
  "d3.min.js",
  "topojson.min.js",
  "datamaps.min.js",
  "jquery.dataTables.min.js",
  "dataTables.bootstrap.js",
  "datetime-moment.js",
  "jquery.ui.widget.js",
  "jquery.fileupload.js",
  "jquery.iframe-transport.js",
  "sweetalert2.min.js",
  "bootstrap-datetime.js",
];

// Libraries pulled from npm (node_modules) instead of manually vendored
// files under static/js/src/vendor/ - the migration target for the rest of
// vendorFiles above. Order matters where a library expects a global (e.g.
// jQuery) to already exist - these all load after vendorFiles above.
const npmVendorFiles = [
  path.join("node_modules", "echarts", "dist", "echarts.min.js"),
  path.join("node_modules", "select2", "dist", "js", "select2.min.js"),
  path.join("node_modules", "papaparse", "papaparse.min.js"),
  path.join("node_modules", "bowser", "es5.js"),
];

// These app scripts don't import anything - they're loaded via plain
// <script> tags (no type="module") and rely on defining top-level `var`s
// and functions as real globals for other scripts/inline handlers to use.
// They must NOT be bundled: esbuild's bundler wraps the file (breaking
// that global-scope contract) and tree-shakes away anything it can't see
// used from within the file itself, which silently guts files like
// autocomplete.js down to nothing.
const appFiles = [
  "autocomplete.js",
  "campaign_results.js",
  "campaigns.js",
  "dashboard.js",
  "groups.js",
  "landing_pages.js",
  "sending_profiles.js",
  "settings.js",
  "templates.js",
  "gophish.js",
  "users.js",
  "webhooks.js",
];

// passwords.js is the one app script with a real import (zxcvbn from
// node_modules), so it genuinely needs bundling to resolve that.
const bundledAppFiles = ["passwords.js"];

const cssFiles = [
  "bootstrap.min.css",
  "main.css",
  "dashboard.css",
  "flat-ui.css",
  "dataTables.bootstrap.css",
  "font-awesome.min.css",
  "bootstrap-datetime.css",
  "checkbox.css",
  "sweetalert2.min.css",
  "select2-bootstrap.min.css",
];

// CSS pulled from npm (node_modules) instead of manually vendored files
const npmCssFiles = [
  path.join("node_modules", "select2", "dist", "css", "select2.min.css"),
];

async function buildVendor() {
  const combined = vendorFiles
    .map((f) => fs.readFileSync(path.join(vendorDir, f), "utf8"))
    .concat(npmVendorFiles.map((f) => fs.readFileSync(f, "utf8")))
    .join("\n;\n");
  const result = await esbuild.transform(combined, {
    loader: "js",
    minify: true,
  });
  fs.mkdirSync(jsDistDir, { recursive: true });
  fs.writeFileSync(path.join(jsDistDir, "vendor.min.js"), result.code);
}

async function buildApp() {
  const outDir = path.join(jsDistDir, "app");
  fs.mkdirSync(outDir, { recursive: true });

  for (const file of appFiles) {
    const name = path.basename(file, ".js");
    const source = fs.readFileSync(path.join(appDir, file), "utf8");
    const result = await esbuild.transform(source, {
      loader: "js",
      target: "es2018",
      minify: true,
    });
    fs.writeFileSync(path.join(outDir, `${name}.min.js`), result.code);
  }

  for (const file of bundledAppFiles) {
    const name = path.basename(file, ".js");
    await esbuild.build({
      entryPoints: [path.join(appDir, file)],
      bundle: true,
      minify: true,
      format: "iife",
      target: "es2018",
      outfile: path.join(outDir, `${name}.min.js`),
      logLevel: "warning",
    });
  }
}

async function buildCSS() {
  const combined = cssFiles
    .map((f) => fs.readFileSync(path.join(cssDir, f), "utf8"))
    .concat(npmCssFiles.map((f) => fs.readFileSync(f, "utf8")))
    .join("\n");
  const result = await esbuild.transform(combined, {
    loader: "css",
    minify: true,
  });
  fs.mkdirSync(cssDistDir, { recursive: true });
  fs.writeFileSync(path.join(cssDistDir, "gophish.css"), result.code);
}

async function main() {
  await buildVendor();
  await buildApp();
  await buildCSS();
  console.log("Build complete.");
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
