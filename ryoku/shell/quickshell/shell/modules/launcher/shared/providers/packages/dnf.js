// Parse `dnf repoquery --installed --available` rows into launcher rows. Each
// line is name, evr, repoid and summary, tab-separated, as QUERY_FORMAT asks.
// dnf lists a package once per repo that carries it, sorted by version, and the
// installed copy comes from the "@System" repo; the rows fold into one per name.
// Pure logic so the provider's parsing is node-tested without dnf.

var QUERY_FORMAT = "%{name}\t%{evr}\t%{repoid}\t%{summary}\n";
var INSTALLED_REPO = "@System";
var LIMIT = 30;

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
    module.exports = { parse, QUERY_FORMAT, INSTALLED_REPO, LIMIT };
}
