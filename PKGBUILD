# Maintainer: Rasmus Steinke <rasi at xssn dot at>
pkgname=('clerkd' 'clerk-rofi' 'clerk-musiclist')
pkgver=1.0
pkgrel=1
arch=('x86_64')
url="https://example.com/clerk"
license=('MIT')
makedepends=('go')
source=("git+https://github.com/carnager/clerk-modular.git")
sha256sums=('SKIP')

build() {
  cd "$srcdir/clerk-modular"
  GOSUMDB=off GOMODCACHE="$srcdir/clerk-modular/.gomodcache" GOCACHE="$srcdir/clerk-modular/.gobuild" \
    ./build
}

package_clerkd() {
  pkgdesc="Clerk API daemon for MPD"
  install -Dm755 "$srcdir/clerk-modular/bin/clerkd" \
                  "$pkgdir/usr/bin/clerkd"
  install -Dm644 "$srcdir/clerk-modular/clerkd/clerkd.service" \
                  "$pkgdir/usr/lib/systemd/user/clerkd.service"
}

package_clerk-rofi() {
  pkgdesc="Rofi client for the Clerk daemon"
  depends=('rofi' 'clerkd')
  install -Dm755 "$srcdir/clerk-modular/bin/clerk-rofi" \
                  "$pkgdir/usr/bin/clerk-rofi"
}

package_clerk-musiclist() {
  pkgdesc="Static music list exporter for Clerk caches"
  depends=('openssh')
  install -Dm755 "$srcdir/clerk-modular/bin/clerk-musiclist" \
                  "$pkgdir/usr/bin/clerk-musiclist"
}
