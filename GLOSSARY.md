# Ryoku

The language of the Ryoku distribution: its desktop, its installers, and the
machines they produce.

## Secure Boot

**Module signing key**:
The one key generated on and kept by each machine, which signs every
out-of-tree kernel module it builds (the NVIDIA driver, xpadneo) and whose
certificate Secure Boot must trust before it loads them. Ryoku holds no module
signing key of its own.
_Avoid_: MOK key, Ryoku key, akmods key

**Enrollment**:
The one-time approval, made at the console in MokManager on the next boot, that
adds a queued module signing key to the machine's trusted keys.
_Avoid_: enrolment, MOK import, key install

**Enrollment password**:
The fixed, publicly documented one-time password MokManager asks for to approve
an enrollment; it proves someone is at the console, not who they are.
_Avoid_: MOK password, Secure Boot password

## Graphics

**Driver fallback**:
A boot in which the machine's NVIDIA kernel module cannot load, so the desktop
comes up on nouveau instead. It lasts until a module exists for the running
kernel; Ryoku reports it and never holds a kernel back to avoid it.
_Avoid_: nouveau mode, safe mode, degraded boot
