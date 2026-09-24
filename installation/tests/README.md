# installation/tests/

Install tests for the Fedora Ryoku installation and ISO compose pipeline.

- `fedora-channels.sh`: verifies repository channel configurations and URLs.
- `fedora-firstboot.sh`: tests the console first-boot setup inside a disposable container.
- `fedora-iso.sh`: validates Kickstart syntax, payload staging, and compose gates.
- `fedora-iso-vm.sh` / `fedora-iso-vm.py`: drives QEMU VM testing of the composed Fedora ISO.
- `fedora-provision.sh`: tests the offline target provisioner (`provision-target.py`).
- `fedora-repo.sh`: verifies pinned GPG keys, repository creation, and dependency closures.
- `fedora-rpm.sh`: verifies package builds, signatures, and installation across compositors.
- `fedora-signatures.sh`: asserts RPM and repository GPG signatures against trusted keys.
