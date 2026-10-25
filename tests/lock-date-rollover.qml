import QtQuick
import Quickshell
ShellRoot {
    id: root
    property var keyboard: ({numLock: false})
    property var config: ({})
    property var sddm: ({hostName: "sandbox", fingerprintHint: true})
    property int phase: 0
    readonly property date yesterday: new Date(Date.now() - 86400000)
    Loader {
        id: theme
        source: "qylock/themes/clockwork/orbital/Main.qml"
    }
    function hasText(item, value) {
        if (item.text !== undefined && item.text === value && item.visible) return true;
        for (var child of item.children || []) if (hasText(child, value)) return true;
        return false;
    }
    function day(d) { return Qt.formatDate(d, "dddd").toUpperCase(); }
    Timer {
        interval: 250; running: true; repeat: true
        onTriggered: {
            if (!theme.item) { console.error("FAIL: theme did not load"); Qt.quit(); return; }
            if (root.phase === 0) {
                if (theme.item.curDate === undefined) {
                    console.error("FAIL: theme has no reactive curDate property"); Qt.quit(); return;
                }
                if (theme.item.softwareCursorEnabled === undefined || theme.item.softwareCursorEnabled) {
                    console.error("FAIL: in-session lock enabled the greeter software cursor"); Qt.quit(); return;
                }
                theme.item.curDate = root.yesterday;
                if (!root.hasText(theme.item, root.day(root.yesterday))) {
                    console.error("FAIL: date label does not follow curDate"); Qt.quit(); return;
                }
                root.phase++;
            } else if (root.phase === 1) {
                if (!root.hasText(theme.item, root.day(root.yesterday))) {
                    console.error("FAIL: lockscreen repainted its clock before one second"); Qt.quit(); return;
                }
                root.phase++;
            } else if (root.phase < 4) {
                root.phase++;
            } else {
                if (!root.hasText(theme.item, root.day(new Date())) || root.hasText(theme.item, root.day(root.yesterday))) {
                    console.error("FAIL: date label stayed on " + root.day(root.yesterday)); Qt.quit(); return;
                }
                console.log("PASS: lockscreen clock rests between one-second updates and rolls the date"); Qt.quit();
            }
        }
    }
}
