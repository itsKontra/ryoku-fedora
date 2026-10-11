.pragma library

// zeroIsReal: a DDC monitor reports 0 as its dimmest backlight, a readable
// level; a backlight device reads 0 while the panel is off or mid-DPMS.
function resolveHardwareBrightness(current, max, lastGood, zeroIsReal) {
    const hasLast = Number.isFinite(lastGood) && lastGood >= 0.01
    const maxOk = Number.isFinite(max) && max > 0
    const currentOk = Number.isFinite(current)
    if (!maxOk || !currentOk) {
        return {
            value: hasLast ? lastGood : Number.NaN,
            restore: hasLast,
            rawMax: maxOk ? max : undefined,
        }
    }
    const normalized = Math.max(0, Math.min(1, current / max))
    if (!zeroIsReal && (current <= 0 || normalized < 0.01)) {
        return {
            value: hasLast ? lastGood : Number.NaN,
            restore: hasLast,
            rawMax: max,
        }
    }
    return {
        value: normalized,
        restore: false,
        rawMax: max,
    }
}

// Hardware level for a 0..1 brightness. Rounded, not truncated, so 0.29 is 29
// and not 28. DDC 0 is the monitor's own minimum and stays lit; a laptop
// backlight at raw 0 turns the panel off on many machines, so it floors at 1.
function rawLevel(value, rawMax, isDdc) {
    if (!Number.isFinite(value) || !Number.isFinite(rawMax) || rawMax <= 0)
        return -1
    const raw = Math.round(Math.max(0, Math.min(1, value)) * rawMax)
    return isDdc ? raw : Math.max(raw, 1)
}

// ddcutil options worth passing on this install, read from its --help, so an
// older ddcutil without them still works. --skip-ddc-checks skips the per-call
// bus probe; on a write --noverify skips the read-back. Measured on ddcutil
// 3.0.2: 0.6-1.1 s per setvcp without them, 0.09 s with them.
function ddcFlags(helpText, forWrite) {
    const text = String(helpText || "")
    const flags = []
    if (text.indexOf("--skip-ddc-checks") >= 0)
        flags.push("--skip-ddc-checks")
    if (forWrite && text.indexOf("--noverify") >= 0)
        flags.push("--noverify")
    return flags
}

function pickRestoreValue(lastGood, currentBrightness) {
    if (Number.isFinite(lastGood) && lastGood >= 0.01)
        return lastGood
    if (Number.isFinite(currentBrightness) && currentBrightness >= 0.01)
        return currentBrightness
    return Number.NaN
}
