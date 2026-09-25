import assert from "node:assert/strict";
import fs from "node:fs";
import test from "node:test";
import vm from "node:vm";

const source = fs.readFileSync(new URL("./Hub.qml", import.meta.url), "utf8");
const clone = value => JSON.parse(JSON.stringify(value));

function harness() {
    const disk = { qsbar: { barShellStyle: "full" }, barStyle: "qsbar", fontSize: 11 };
    const patches = [];
    const hub = {
        defs: clone(disk), draft: {}, committed: {}, pristine: true,
        liveKeys: Object.keys(disk), liveBaseline: null, livePending: {}, liveEdited: {},
        hyprCommitted: {}, hyprChanges: () => [], savePage() {}, revertPage() {},
        invalidatePageWork() {}, requestReloadCoverPrune() {}
    };
    const context = {
        hub, defs: hub.defs, srcOf: {}, I18n: {},
        liveFlush: { restart() {}, stop() {} },
        Settings: {
            ready: true,
            get: key => clone(disk[key]),
            patch(key, value) { patches.push({ key, value: clone(value) }); disk[key] = clone(value); }
        }
    };
    for (const key of ["draft", "committed"])
        Object.defineProperty(context, key, { get: () => hub[key] });
    vm.createContext(context);
    for (const name of ["snapshot", "rebase", "val", "edit", "captureLiveBaseline", "stageLive", "restoreLiveUnsaved", "save", "revert"]) {
        const match = source.match(new RegExp("^    function " + name + "\\([^\\n]*\\{(?:[^\\n]*\\}$|[\\s\\S]*?^    \\})", "m"));
        assert.ok(match, name + " exists in Hub.qml");
        vm.runInContext(match[0], context);
        hub[name] = context[name];
    }
    const changes = source.match(/readonly property var liveChanges: \{([\s\S]*?)\n    \}/)[1];
    Object.defineProperty(hub, "liveChanges", { get: () => vm.runInContext("(function() {" + changes + "})()", context) });
    hub.rebase();
    return { hub, disk, patches, externalForm(form) {
        disk.qsbar.barShellStyle = form;
        hub.rebase();
    }, flush() {
        for (const key of Object.keys(hub.livePending)) context.Settings.patch(key, hub.val(key));
        hub.livePending = {};
        hub.rebase();
    } };
}

for (const action of ["save", "revert", "restoreLiveUnsaved"]) {
    test(`external bar form survives ${action} with an unrelated Hub draft`, () => {
        const h = harness();
        h.hub.edit("fontSize", 14);
        h.externalForm("islands");
        assert.equal(h.hub.val("fontSize"), 14);
        assert.equal(h.hub.val("qsbar").barShellStyle, "islands");
        h.hub[action]();
        assert.equal(h.disk.qsbar.barShellStyle, "islands");
        assert.ok(h.patches.every(patch => patch.key !== "qsbar"));
    });
}

test("external bar changes are not unsaved Hub edits even in a pristine Hub", () => {
    const h = harness();
    h.externalForm("dock");
    assert.equal(h.hub.liveChanges.length, 0);
    h.hub.restoreLiveUnsaved();
    assert.equal(h.patches.length, 0);
});

test("live preview remains revertible after its echo and external bar updates", () => {
    const h = harness();
    h.hub.stageLive("fontSize", 14);
    h.externalForm("islands");
    h.flush();
    h.hub.rebase();
    assert.equal(h.hub.liveBaseline.fontSize, 11);
    h.hub.restoreLiveUnsaved();
    assert.equal(h.disk.fontSize, 11);
    assert.equal(h.disk.qsbar.barShellStyle, "islands");
});

test("save accepts live previews and later external edits become the new baseline", () => {
    const h = harness();
    h.hub.stageLive("fontSize", 14);
    h.flush();
    h.hub.save();
    h.disk.fontSize = 16;
    h.hub.rebase();
    h.externalForm("notch");
    h.hub.restoreLiveUnsaved();
    assert.equal(h.disk.fontSize, 16);
    assert.equal(h.disk.qsbar.barShellStyle, "notch");
});
