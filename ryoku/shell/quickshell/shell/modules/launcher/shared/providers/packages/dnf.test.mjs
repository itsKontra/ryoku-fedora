import { createRequire } from "node:module";
const require = createRequire(import.meta.url);
const { parse, searchCommand, QUERY_FORMAT, LIMIT } = require("./dnf.js");

let failed = 0;
function eq(actual, expected, msg) {
    const a = JSON.stringify(actual);
    const e = JSON.stringify(expected);
    if (a === e) console.log("PASS " + msg);
    else { failed++; console.log("FAIL " + msg + "\n  expected " + e + "\n  got      " + a); }
}

const rows = [
    "ripgrep-edit\t0.3.17-2.fc44\tupdates\tEdit ripgrep search results",
    "ripgrep\t14.1.1-4.fc44\tfedora\tLine-oriented search tool",
    "ripgrep\t15.2.0-1.fc44\t@System\tLine-oriented search tool",
    "ripgrep\t15.2.0-1.fc44\tupdates\tLine-oriented search tool",
    "emacs-ripgrep\t1.0-1.fc44\tfedora\tEmacs front end",
    ""
].join("\n");

const got = parse(rows, "ripgrep");
eq(got.map(p => p.name), ["ripgrep", "ripgrep-edit", "emacs-ripgrep"], "exact, then prefix, then substring");
eq(got[0], { name: "ripgrep", version: "15.2.0-1.fc44", source: "updates", description: "Line-oriented search tool", installed: true }, "repo rows fold into one installed row at the newest version");
eq(got[1].installed, false, "a package with no @System row is not installed");

eq(parse("local-only\t1.0-1\t@System\tBuilt by hand", "local")[0],
    { name: "local-only", version: "", source: "installed", description: "Built by hand", installed: true },
    "an installed package no repo carries still shows");

eq(parse("", "x"), [], "no output yields no rows");
eq(parse("Updating and loading repositories:\nnot\ta row", "x"), [], "noise lines are dropped");

const many = Array.from({ length: LIMIT + 5 }, (_, i) => "pkg" + i + "\t1\tfedora\ts").join("\n");
eq(parse(many, "pkg").length, LIMIT, "results are capped");

eq(searchCommand("  screenshot   tool ", true).slice(3), ["sh", QUERY_FORMAT, "-C", "screenshot", "tool"], "words become separate arguments, cache-only flagged");
eq(searchCommand("kitty", false).slice(4), [QUERY_FORMAT, "", "kitty"], "the full pass drops -C");
eq(searchCommand("$(rm -rf ~)", true).slice(-3), ["$(rm", "-rf", "~)"], "a hostile term stays data, never script");

if (failed > 0) { console.log("\n" + failed + " test(s) FAILED"); process.exit(1); }
console.log("\nAll tests PASSED");
