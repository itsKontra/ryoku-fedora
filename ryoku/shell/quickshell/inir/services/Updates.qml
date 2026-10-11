pragma Singleton

import inir.modules.common
import inir.modules.common.functions
import QtQuick
import Quickshell
import Quickshell.Io

/*
 * System updates service: counts the DNF upgrades waiting for this machine.
 */
Singleton {
    id: root

    property bool available: false
    property int count: 0
    
    readonly property bool updateAdvised: available && count > (Config.options?.updates?.adviseUpdateThreshold ?? 75)
    readonly property bool updateStronglyAdvised: available && count > (Config.options?.updates?.stronglyAdviseUpdateThreshold ?? 200)

    function load() {}
    function refresh() {
        if (!available) return;
        print("[Updates] Checking for system updates")
        if (checkUpdatesProc.running) return;
        root._checkOutput = null
        root._checkExit = -1
        checkUpdatesProc.running = true;
    }

    Timer {
        interval: (Config.options?.updates?.checkInterval ?? 120) * 60 * 1000
        repeat: true
        running: Config.ready
        onTriggered: {
            print("[Updates] Periodic update check due")
            root.refresh();
        }
    }

    Timer {
        id: availabilityDefer
        interval: 1500
        repeat: false
        onTriggered: checkAvailabilityProc.running = true
    }

    Connections {
        target: Config
        function onReadyChanged() {
            if (Config.ready) availabilityDefer.start()
        }
    }

    Component.onCompleted: if (Config.ready) availabilityDefer.start()

    Process {
        id: checkAvailabilityProc
        running: false
        command: ["/usr/bin/bash", "-c", "command -v dnf5 &>/dev/null || command -v dnf &>/dev/null"]
        onExited: (exitCode, exitStatus) => {
            root.available = exitCode === 0
            root.refresh();
        }
    }

    // The check's output and exit code arrive in either order; the count settles once both are in.
    property var _checkOutput: null
    property int _checkExit: -1
    function _settleCheck(): void {
        if (root._checkOutput === null || root._checkExit < 0) return
        const lines = String(root._checkOutput).trim()
        const listed = lines.length > 0 ? lines.split("\n").length : 0
        const code = root._checkExit
        root._checkOutput = null
        root._checkExit = -1
        // A check that could not run (offline, metadata locked) keeps the last count instead of announcing
        // an up-to-date system.
        if (code === 0) {
            root.count = listed
        } else {
            const reason = (checkUpdatesErr.text ?? "").trim().split("\n").pop()
            console.info("[Updates] could not check for DNF updates:", reason || `repoquery exited ${code}`)
        }
    }

    Process {
        id: checkUpdatesProc
        command: ["/usr/bin/bash", "-c",
            "set -o pipefail; dnf_cmd=$(command -v dnf5 || command -v dnf) || exit 127; " +
            "\"$dnf_cmd\" -q repoquery --upgrades --qf '%{name}.%{arch}\\n' | sed '/^$/d' | sort -u"
        ]
        stdout: StdioCollector {
            onStreamFinished: {
                root._checkOutput = text ?? ""
                root._settleCheck()
            }
        }
        stderr: StdioCollector {
            id: checkUpdatesErr
        }
        onExited: (exitCode, exitStatus) => {
            root._checkExit = exitCode
            root._settleCheck()
        }
    }
}
