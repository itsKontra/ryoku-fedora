pragma Singleton
pragma ComponentBehavior: Bound

import inir.modules.common
import inir.modules.common.functions
import inir.services
import QtQuick
import Quickshell
import Quickshell.Io

/**
 * PackageSearch — Async package manager search service.
 *
 * Searches the DNF repositories for packages matching a query.
 * Results are parsed into structured objects with name, version, repo,
 * description, and installed status.
 *
 * Usage:
 *   PackageSearch.search("vesktop")
 *   // results available in PackageSearch.results after search completes
 */
Singleton {
    id: root

    property string query: ""
    property bool searching: false
    property var results: []
    property string error: ""

    // Debounce to avoid spamming package manager
    property int debounceMs: 300

    function _safeTerminal(): string {
        const configured = (Config.options?.apps?.terminal ?? "kitty").trim()
        if (configured.length === 0)
            return "kitty"
        if (!/^[A-Za-z0-9._+-]+$/.test(configured))
            return "kitty"
        return configured
    }

    function _runTerminalScript(script: string, args): void {
        const command = ["/usr/bin/bash", "-lc", script + "\nprintf \"\\nPress Enter to close...\"\nread", "bash", ...(args ?? [])]
        const terminal = root._safeTerminal()
        if (terminal === "wezterm") {
            ShellExec.execDetachedArgs([terminal, "start", "--always-new-process", "--", ...command], "Run package action")
            return
        }
        ShellExec.execDetachedArgs([terminal, "-e", ...command], "Run package action")
    }

    function isSafePackageName(name: string): bool {
        const pkg = (name ?? "").trim()
        return pkg.length > 0 && /^[A-Za-z0-9@._+-]+$/.test(pkg)
    }

    function installPackage(name: string, preferAurHelper: bool): bool {
        const pkg = (name ?? "").trim()
        if (!root.isSafePackageName(pkg)) {
            Quickshell.execDetached(["/usr/bin/notify-send", Translation.tr("Install Package"),
                Translation.tr("Invalid package name"), "-a", "Shell"])
            return false
        }

        // Fedora has no AUR; an AUR-helper request installs from the repositories.
        root._runTerminalScript("sudo ryoku-host pkg install \"$1\"", [pkg])
        return true
    }

    function removePackage(name: string): bool {
        const pkg = (name ?? "").trim()
        if (!root.isSafePackageName(pkg)) {
            Quickshell.execDetached(["/usr/bin/notify-send", Translation.tr("Remove Package"),
                Translation.tr("Invalid package name"), "-a", "Shell"])
            return false
        }

        root._runTerminalScript("sudo ryoku-host pkg remove \"$1\"", [pkg])
        return true
    }

    function updateSystem(): void {
        root._runTerminalScript("ryoku update", [])
    }

    // apps.update is a person's own command and is kept intact; empty, or an
    // Arch-era default carried over in a config, runs Ryoku's own update.
    function runConfiguredUpdate(): void {
        const cmd = (Config.options?.apps?.update ?? "").trim()
        if (cmd.length === 0 || /\bpacman\b|arch-update|\byay\b|\bparu\b/.test(cmd)) {
            root.updateSystem()
            return
        }
        ShellExec.execCmd(cmd)
    }

    function cleanPackageCache(): void {
        root._runTerminalScript("dnf_cmd=$(command -v dnf5 || command -v dnf) || exit 127; sudo \"$dnf_cmd\" clean packages", [])
    }

    function _dnfSearchPipeline(installedOnly: bool, limit: int): string {
        const source = installedOnly
            ? "rpm -qa --qf '%{NAME}\\t%{VERSION}-%{RELEASE}\\t@System\\t%{SUMMARY}\\n' | grep -iF -- \"$1\""
            : "dnf_cmd=$(command -v dnf5 || command -v dnf) || exit 127; " +
                "\"$dnf_cmd\" -q repoquery --available --qf '%{name}\\t%{evr}\\t%{repoid}\\t%{summary}\\n' \"*$1*\""
        return source + " | sed '/^$/d' | head -" + limit + " | while IFS=$'\\t' read -r name version repo desc; do [ -n \"$name\" ] || continue; " +
            "installed=0; rpm -q --quiet \"$name\" && installed=1; " +
            "printf '__DNF__\\t%s\\t%s\\t%s\\t%s\\t%s\\n' \"$installed\" \"$name\" \"$version\" \"$repo\" \"$desc\"; " +
            "done"
    }

    function search(q: string): void {
        root.query = q.trim()
        if (root.query === "") {
            root.results = []
            root.searching = false
            root.error = ""
            _debounceTimer.stop()
            return
        }
        _debounceTimer.restart()
    }

    function clear(): void {
        root.query = ""
        root.results = []
        root.searching = false
        root.error = ""
        _debounceTimer.stop()
        _searchProc.running = false
    }

    Timer {
        id: _debounceTimer
        interval: root.debounceMs
        onTriggered: {
            if (root.query === "") return
            root.searching = true
            root.error = ""
            _stdout = ""
            _searchProc.command = ["/usr/bin/bash", "-lc", root._dnfSearchPipeline(false, 200), "bash", root.query]
            _searchProc.running = true
        }
    }

    property string _stdout: ""

    Process {
        id: _searchProc
        stdout: SplitParser {
            splitMarker: ""
            onRead: data => { root._stdout += data }
        }
        onExited: (exitCode, exitStatus) => {
            root.searching = false
            if (exitCode !== 0 && root._stdout.trim() === "") {
                root.results = []
                return
            }
            root.results = root._parseResults(root._stdout)
        }
    }

    // Timeout for slow searches
    Timer {
        id: _timeoutTimer
        interval: 15000
        running: _searchProc.running
        onTriggered: {
            _searchProc.running = false
            root.searching = false
            root.error = "Search timed out"
        }
    }

    function _parseResults(output: string): list<var> {
        const lines = output.split("\n")
        const pkgs = []
        let i = 0
        while (i < lines.length) {
            const line = lines[i]
            if (line.startsWith("__DNF__\t")) {
                const fields = line.split("\t")
                if (fields.length >= 6) {
                    pkgs.push({
                        name: fields[2],
                        version: fields[3],
                        repo: fields[4],
                        description: fields.slice(5).join("\t"),
                        installed: fields[1] === "1",
                        votes: 0,
                        popularity: 0,
                        isAur: false
                    })
                }
                i++
                continue
            }
            i++
        }
        return pkgs
    }

    // Search for installed packages only (for remove operations)
    function searchInstalled(q: string): void {
        root.query = q.trim()
        if (root.query === "") {
            root.results = []
            root.searching = false
            return
        }
        root.searching = true
        root.error = ""
        _stdout = ""
        _installedProc.command = ["/usr/bin/bash", "-lc", root._dnfSearchPipeline(true, 100), "bash", root.query]
        _installedProc.running = true
    }

    Process {
        id: _installedProc
        stdout: SplitParser {
            splitMarker: ""
            onRead: data => { root._stdout += data }
        }
        onExited: (exitCode, exitStatus) => {
            root.searching = false
            if (exitCode !== 0 && root._stdout.trim() === "") {
                root.results = []
                return
            }
            // Parse results and mark all as installed
            const parsed = root._parseResults(root._stdout)
            root.results = parsed.map(pkg => Object.assign({}, pkg, { installed: true }))
        }
    }

    IpcHandler {
        target: "packageSearch"

        function search(query: string): string {
            root.search(query)
            return "searching: " + query
        }

        function results(): string {
            return root.results.map(p =>
                `${p.repo}/${p.name} ${p.version}${p.installed ? " [installed]" : ""}\t${p.description}`
            ).join("\n")
        }
    }
}
