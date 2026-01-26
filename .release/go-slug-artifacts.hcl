schema = 1
artifacts {
  zip = [
    "go-slug_${version}_darwin_amd64.zip",
    "go-slug_${version}_darwin_arm64.zip",
    "go-slug_${version}_freebsd_386.zip",
    "go-slug_${version}_freebsd_amd64.zip",
    "go-slug_${version}_freebsd_arm.zip",
    "go-slug_${version}_linux_386.zip",
    "go-slug_${version}_linux_amd64.zip",
    "go-slug_${version}_linux_arm.zip",
    "go-slug_${version}_linux_arm64.zip",
    "go-slug_${version}_netbsd_386.zip",
    "go-slug_${version}_netbsd_amd64.zip",
    "go-slug_${version}_netbsd_arm.zip",
    "go-slug_${version}_openbsd_386.zip",
    "go-slug_${version}_openbsd_amd64.zip",
    "go-slug_${version}_openbsd_arm.zip",
    "go-slug_${version}_solaris_amd64.zip",
    "go-slug_${version}_windows_386.zip",
    "go-slug_${version}_windows_amd64.zip",
  ]
  rpm = [
    "go-slug-${version_linux}-1.aarch64.rpm",
    "go-slug-${version_linux}-1.armv7hl.rpm",
    "go-slug-${version_linux}-1.i386.rpm",
    "go-slug-${version_linux}-1.x86_64.rpm",
  ]
  deb = [
    "go-slug_${version_linux}-1_amd64.deb",
    "go-slug_${version_linux}-1_arm64.deb",
    "go-slug_${version_linux}-1_armhf.deb",
    "go-slug_${version_linux}-1_i386.deb",
  ]
}