#!/usr/bin/env node
// Audits the licences of everything in node_modules.
//
// LICENCE.md restricts a field of endeavour. That cannot lawfully be done to a
// work incorporating GPL, LGPL, AGPL or SSPL code, so a single copyleft
// dependency anywhere in this tree would make the project's own licence
// unenforceable as written. DECISIONS.md says to re-run the audit whenever the
// lockfile changes; this is the thing that runs.
//
// No dependencies of its own, for the same reason.

import { existsSync, readFileSync, readdirSync, statSync } from "node:fs";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const root = join(dirname(fileURLToPath(import.meta.url)), "..");
const repo = join(root, "..");

// Everywhere in this repository that npm packages can end up. Prism gadgets may
// declare their own dependencies, which would otherwise install into the gadget
// folder and never be audited.
function moduleRoots() {
  const roots = [join(root, "node_modules")];
  const gadgets = join(repo, ".prism", "gadgets");
  if (existsSync(gadgets)) {
    for (const name of readdirSync(gadgets)) {
      const candidate = join(gadgets, name, "node_modules");
      if (existsSync(candidate)) roots.push(candidate);
    }
  }
  return roots.filter((dir) => existsSync(dir));
}

/** Licences that can be combined with a field-of-use restriction. */
const PERMITTED = new Set([
  "MIT",
  "MIT-0",
  "ISC",
  "Apache-2.0",
  "BSD-2-Clause",
  "BSD-3-Clause",
  "0BSD",
  "CC0-1.0",
  "CC-BY-4.0",
  "Unlicense",
  "Python-2.0",
  "BlueOak-1.0.0",
  "WTFPL",
]);

/** Licences that cannot. The reason is the whole point of the audit. */
const FORBIDDEN = [
  { match: /\bAGPL/i, why: "network copyleft" },
  { match: /\bLGPL/i, why: "weak copyleft" },
  { match: /\bGPL/i, why: "copyleft" },
  { match: /\bSSPL/i, why: "service-side copyleft" },
  { match: /\bOSL/i, why: "copyleft" },
  { match: /\bEUPL/i, why: "copyleft" },
  { match: /\bCDDL/i, why: "file-level copyleft" },
  { match: /\bMPL/i, why: "file-level copyleft" },
  { match: /\bCC-BY-SA/i, why: "share-alike" },
  { match: /\bCC-BY-NC/i, why: "non-commercial" },
];

function packageDirs(dir) {
  const out = [];
  let entries;
  try {
    entries = readdirSync(dir);
  } catch {
    return out;
  }
  for (const name of entries) {
    if (name === ".bin" || name === ".cache") continue;
    const full = join(dir, name);
    if (!statSync(full).isDirectory()) continue;
    if (name.startsWith("@")) {
      out.push(...packageDirs(full));
      continue;
    }
    out.push(full);
    out.push(...packageDirs(join(full, "node_modules")));
  }
  return out;
}

/** Normalises the many shapes a package.json licence field comes in. */
function licenceOf(pkg) {
  if (typeof pkg.license === "string") return pkg.license;
  if (pkg.license && typeof pkg.license.type === "string") return pkg.license.type;
  if (Array.isArray(pkg.licenses)) {
    return pkg.licenses.map((l) => (typeof l === "string" ? l : l.type)).join(" OR ");
  }
  return "";
}

/** Splits "(MIT OR Apache-2.0)" into its terms. */
function terms(expression) {
  return expression
    .replace(/[()]/g, " ")
    .split(/\s+(?:OR|AND|WITH)\s+/i)
    .map((t) => t.trim())
    .filter(Boolean);
}

const problems = [];
const unknown = [];
const counts = new Map();
let audited = 0;

const roots = moduleRoots();
const everyPackage = roots.flatMap((dir) => packageDirs(dir));

for (const dir of everyPackage) {
  let pkg;
  try {
    pkg = JSON.parse(readFileSync(join(dir, "package.json"), "utf8"));
  } catch {
    continue;
  }
  if (!pkg.name) continue;
  audited++;

  const expression = licenceOf(pkg).trim();
  const label = expression || "(none declared)";
  counts.set(label, (counts.get(label) ?? 0) + 1);

  const forbidden = FORBIDDEN.find((f) => f.match.test(expression));
  if (forbidden) {
    // A choice of licences is fine as long as one option is usable.
    const escape = terms(expression).find((t) => PERMITTED.has(t));
    if (!escape) {
      problems.push({ name: pkg.name, version: pkg.version, expression, why: forbidden.why });
      continue;
    }
  }

  if (!expression || !terms(expression).some((t) => PERMITTED.has(t))) {
    unknown.push({ name: pkg.name, version: pkg.version, expression: label });
  }
}

if (audited === 0) {
  console.error("No packages found. Run npm install first.");
  process.exit(2);
}

console.log(
  `Audited ${audited} installed packages in ${roots.length} ${roots.length === 1 ? "place" : "places"}:`,
);
for (const dir of roots) console.log(`  ${dir.replace(repo, ".")}`);
for (const [licence, count] of [...counts].sort((a, b) => b[1] - a[1])) {
  console.log(`  ${String(count).padStart(4)}  ${licence}`);
}

if (unknown.length > 0) {
  console.log("\nNot on the permitted list — check these by hand:");
  for (const u of unknown) console.log(`  ${u.name}@${u.version}: ${u.expression}`);
}

if (problems.length > 0) {
  console.error("\nIncompatible with LICENCE.md:");
  for (const p of problems) {
    console.error(`  ${p.name}@${p.version}: ${p.expression} (${p.why})`);
  }
  console.error(
    "\nThe Aerial Public Licence restricts a field of endeavour, which cannot be combined\n" +
      "with copyleft terms. Remove the dependency or change the project licence.",
  );
  process.exit(1);
}

if (unknown.length > 0) {
  console.error("\nStopping: some licences were not recognised. Add them to the list once checked.");
  process.exit(1);
}

console.log("\nEvery dependency licence is compatible with LICENCE.md.");
