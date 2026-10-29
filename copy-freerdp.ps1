# Run in administrator PowerShell. Copies the runtime set from the tested
# MSYS2 UCRT64 build; dependency license collection is separate before shipping.
param(
    [string]$Source = 'C:\msys64\ucrt64\bin',
    [string]$Destination = 'C:\Program Files\FreeRDP'
)

$ErrorActionPreference = 'Stop'
$files = @'
sdl-freerdp.exe
libfreerdp-client3.dll
libusb-1.0.dll
libfreerdp3.dll
avcodec-63.dll
libaom.dll
libwinpthread-1.dll
libcairo-2.dll
libgcc_s_seh-1.dll
libstdc++-6.dll
libfontconfig-1.dll
libintl-8.dll
libiconv-2.dll
libexpat-1.dll
libfreetype-6.dll
libbrotlidec.dll
libbrotlicommon.dll
libbz2-1.dll
libharfbuzz-0.dll
libglib-2.0-0.dll
libpcre2-8-0.dll
libgraphite2.dll
libpng16-16.dll
zlib1.dll
libpixman-1-0.dll
libdav1d-7.dll
libgobject-2.0-0.dll
libffi-8.dll
libgsm.dll
libjxl.dll
libbrotlienc.dll
libhwy.dll
libjxl_cms.dll
liblcms2-2.dll
libjxl_threads.dll
liblc3-1.dll
liblzma-5.dll
libmp3lame-0.dll
libmpg123-0.dll
libopencore-amrnb-0.dll
libopencore-amrwb-0.dll
libopenjp2-7.dll
libopus-0.dll
librav1e.dll
librsvg-2-2.dll
libcairo-gobject-2.dll
libgdk_pixbuf-2.0-0.dll
libgio-2.0-0.dll
libgmodule-2.0-0.dll
libjpeg-8.dll
libtiff-6.dll
libdeflate.dll
libjbig-0.dll
libLerc.dll
libwebp-7.dll
libsharpyuv-0.dll
libzstd.dll
libpango-1.0-0.dll
libfribidi-0.dll
libthai-0.dll
libdatrie-1.dll
libpangocairo-1.0-0.dll
libpangoft2-1.0-0.dll
libpangowin32-1.0-0.dll
libxml2-16.dll
libspeex-1.dll
libSvtAv1Enc-4.dll
libtheoradec-2.dll
libtheoraenc-2.dll
libogg-0.dll
libva.dll
libvorbis-0.dll
libvorbisenc-2.dll
libvpl-2.dll
libvpx-1.dll
libwebpmux-3.dll
libx264-165.dll
libx265-217.dll
xvidcore.dll
libzvbi-0.dll
avutil-61.dll
libva_win32.dll
swresample-7.dll
libsoxr.dll
libgomp-1.dll
libcrypto-3-x64.dll
libssl-3-x64.dll
swscale-10.dll
libwinpr3.dll
libcjson-1.dll
liburiparser-1.dll
SDL3.dll
SDL3_ttf.dll
'@ -split '\r?\n'

# Preflight before writing anything, so an updated MSYS2 package can't silently
# produce an incomplete bundle with this version-specific manifest.
$missing = @($files | Where-Object {
    -not (Test-Path -LiteralPath (Join-Path $Source $_) -PathType Leaf)
})
if ($missing.Count -gt 0) {
    throw "Missing files in ${Source}: $($missing -join ', '). Nothing copied."
}

New-Item -ItemType Directory -Force -Path $Destination | Out-Null
foreach ($file in $files) {
    Copy-Item -LiteralPath (Join-Path $Source $file) -Destination $Destination
}
Write-Host "Copied $($files.Count) runtime files to $Destination"
Write-Host "Test: & '$Destination\sdl-freerdp.exe' /help"
