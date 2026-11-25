# fix-windows-firefox-install

Backlog B4: omnisnap install firefox fails on Windows because Mozilla's MSI holds no files (it wraps the setup .exe). Download the signed .exe, carve out its 7z archive and extract it with tar.exe or 7-Zip. Blocks v0.2.0.
