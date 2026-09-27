// Package search through dnf. `dnf search` matches every word against names and
// summaries but prints neither versions nor what is installed, so its names (in
// its own best-first order) feed `dnf repoquery --installed --available`. Each
// repoquery line is name, evr, repoid and summary, tab-separated; dnf lists a
// package once per repo that carries it, sorted by version, and the installed
// copy comes from the "@System" repo, so the rows fold into one per name.
// Pure logic so the provider's parsing is node-tested without dnf.

var QUERY_FORMAT = "%{name}\t%{evr}\t%{repoid}\t%{summary}\n";
var INSTALLED_REPO = "@System";
var LIMIT = 30;
var CANDIDATES = 200;

var SEARCH_SCRIPT = 'qf=$1; cache=$2; shift 2; '
    + 'dnf search -q $cache -- "$@" '
    + '| awk -F"\t" \'/^ /{n=$1; sub(/^ +/,"",n); sub(/\\.[^.]*$/,"",n); if(!seen[n]++) print n}\' '
    + '| head -n ' + CANDIDATES + ' '
    + '| xargs -r dnf repoquery -q $cache --installed --available --qf "$qf" --';

// argv for one search. cacheOnly answers from dnf's metadata cache (-C) without
// touching the network; the words go in as arguments, never into the script.
function searchCommand(term, cacheOnly) {
    var words = String(term || "").trim().split(/\s+/).filter(function (w) { return w.length > 0; });
    return ["sh", "-c", SEARCH_SCRIPT, "sh", QUERY_FORMAT, cacheOnly ? "-C" : ""].concat(words);
}

function rank(name, term) {
    var n = name.toLowerCase();
    var t = term.toLowerCase();
    if (n === t)
        return 0;
    if (n.indexOf(t) === 0)
        return 1;
    return 2;
}

function parse(raw, term) {
    var byName = {};
    var order = [];
    var lines = String(raw || "").split("\n");
    for (var i = 0; i < lines.length; i++) {
        var f = lines[i].split("\t");
        if (f.length < 4 || !f[0])
            continue;
        var p = byName[f[0]];
        if (!p) {
            p = byName[f[0]] = { name: f[0], version: "", source: "", description: f[3], installed: false };
            order.push(p);
        }
        if (f[2] === INSTALLED_REPO) {
            p.installed = true;
        } else {
            p.version = f[1];
            p.source = f[2];
        }
    }
    order.forEach(function (p) {
        if (!p.source)
            p.source = "installed";
    });
    var t = String(term || "");
    order.sort(function (a, b) {
        return rank(a.name, t) - rank(b.name, t) || (a.name < b.name ? -1 : a.name > b.name ? 1 : 0);
    });
    return order.slice(0, LIMIT);
}

if (typeof module !== "undefined" && module.exports) {
    module.exports = { parse, searchCommand, QUERY_FORMAT, INSTALLED_REPO, LIMIT };
}
