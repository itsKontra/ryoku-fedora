# External dependencies

Some tools Ryoku uses have no RPM source yet (the package-porting gaps listed in
[`installation/fedora/README.md`](../installation/fedora/README.md)). Until
they are packaged, this page is where to look up how to install one by hand.
Each entry names what needs it, where Ryoku expects it, and the commands.

A manual install lives outside dnf, so `ryoku update` never upgrades it.
Delete an entry here once the tool ships as a package.

## waifu2x-ncnn-vulkan

The AI upscaler behind ryoshot Beautify HD and ryowalls Enhance. Ryoku looks up
the binary on `PATH` and loads models from
`/usr/share/waifu2x-ncnn-vulkan/models-cunet`
(`ryoku/shell/ryogami/daemon/upscale.go`,
`ryoku/shell/quickshell/ryoshot/Beautify.qml`). Without it, both features
report that waifu2x-ncnn-vulkan is not installed.

Download the Linux release zip from
[nihui/waifu2x-ncnn-vulkan](https://github.com/nihui/waifu2x-ncnn-vulkan/releases)
and unpack it, then install the binary and the models:

```bash
D=~/Downloads/waifu2x-ncnn-vulkan-20250915-linux
sudo install -Dm755 "$D/waifu2x-ncnn-vulkan" /usr/local/bin/waifu2x-ncnn-vulkan
sudo mkdir -p /usr/share/waifu2x-ncnn-vulkan
sudo cp -r "$D/models-cunet" "$D/models-upconv_7_anime_style_art_rgb" \
  "$D/models-upconv_7_photo" /usr/share/waifu2x-ncnn-vulkan/
sudo restorecon -R /usr/local/bin/waifu2x-ncnn-vulkan /usr/share/waifu2x-ncnn-vulkan
```

`restorecon` gives the copied files their SELinux labels; without it the
binary can be denied when the shell launches it. Check the install with
`waifu2x-ncnn-vulkan -h`.

To remove it:

```bash
sudo rm -rf /usr/local/bin/waifu2x-ncnn-vulkan /usr/share/waifu2x-ncnn-vulkan
```
