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

// A single ordered list, since load order matters
const vendorFiles = [
  path.join("node_modules", "jquery", "dist", "jquery.min.js"),
  path.join("node_modules", "bootstrap", "dist", "js", "bootstrap.min.js"),
  path.join("node_modules", "moment", "min", "moment.min.js"),
  "d3.min.js",
  "topojson.min.js",
  "datamaps.min.js",
  path.join("node_modules", "datatables.net", "js", "jquery.dataTables.min.js"),
  path.join("node_modules", "datatables.net-bs", "js", "dataTables.bootstrap.min.js"),
  "datetime-moment.js",
  "bootstrap-datetime.js",
  path.join("node_modules", "echarts", "dist", "echarts.min.js"),
  path.join("node_modules", "select2", "dist", "js", "select2.min.js"),
  path.join("node_modules", "papaparse", "papaparse.min.js"),
  path.join("node_modules", "bowser", "es5.js"),
  path.join("node_modules", "sweetalert2", "dist", "sweetalert2.min.js"),
];

function resolveVendorFile(f) {
  return f.startsWith("node_modules" + path.sep) ? f : path.join(vendorDir, f);
}

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

// A single ordered list, since cascade order matters (e.g. main.css must
// follow bootstrap.min.css to override it)
const cssFiles = [
  path.join("node_modules", "bootstrap", "dist", "css", "bootstrap.min.css"),
  "main.css",
  "dashboard.css",
  "flat-ui.css",
  path.join("node_modules", "datatables.net-bs", "css", "dataTables.bootstrap.min.css"),
  "font-awesome.min.css",
  "bootstrap-datetime.css",
  "checkbox.css",
  "select2-bootstrap.min.css",
  path.join("node_modules", "select2", "dist", "css", "select2.min.css"),
  path.join("node_modules", "sweetalert2", "dist", "sweetalert2.min.css"),
];

function resolveCssFile(f) {
  return f.startsWith("node_modules" + path.sep) ? f : path.join(cssDir, f);
}

// The npm bootstrap package's CSS references its glyphicon fonts as
// "../fonts/..." (relative to dist/css/), assuming it's deployed as its own
// dist/ tree. We serve everything under static/ from one root instead, with
// the fonts already in static/font/ (singular) alongside font-awesome's, so
// rewrite the reference to match once the file is pulled out of that tree.
function fixBootstrapFontPaths(content, file) {
  return file.includes("bootstrap")
    ? content.replace(/\.\.\/fonts\//g, "/font/")
    : content;
}

async function buildVendor() {
  const combined = vendorFiles
    .map((f) => fs.readFileSync(resolveVendorFile(f), "utf8"))
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
    .map((f) => fixBootstrapFontPaths(fs.readFileSync(resolveCssFile(f), "utf8"), f))
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
