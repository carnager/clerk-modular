# Maintainer: Rasmus Steinke <rasi at xssn dot at>
pkgname=('clerk-core' 'clerk-rofi' 'clerk-web')
pkgver=1.0
pkgrel=1
arch=('x86_64')
url="https://example.com/clerk"
license=('MIT')
provides=('clerk-core' 'clerk-rofi' 'clerk-web')
conflicts=()

# Using a Git source array for the repository, assuming all files are within it
source=(
  "git+https://github.com/carnager/clerk-modular.git"
)
sha256sums=('SKIP') # Only one source entry now, the git repo itself.

package_clerk-core() {
  pkgdesc="Core library for MPD‐based music rating & control"
  depends=('python' 'python-mpd2' 'python-msgpack' 'python-toml')
  local sp
  sp="$(python3 -c 'import sysconfig; print(sysconfig.get_paths()["purelib"])')"
  install -Dm644 "$srcdir/clerk-modular/clerk_core.py" \
                  "$pkgdir/$sp/clerk_core.py"
}

package_clerk-rofi() {
  pkgdesc="Rofi UI frontend for clerk-core"
  depends=('rofi' 'clerk-core')
  install -Dm755 "$srcdir/clerk-modular/clerk-rofi" \
                  "$pkgdir/usr/bin/clerk-rofi"
  install -Dm755 "$srcdir/clerk-modular/clerk-api-rofi" \
                  "$pkgdir/usr/bin/clerk-api-rofi"
}

package_clerk-web() {
  pkgdesc="Flask web service for clerk-core"
  depends=('python-flask' 'clerk-core')

  # Service executable
  install -Dm755 "$srcdir/clerk-modular/clerk-service" \
                  "$pkgdir/usr/bin/clerk-service"

  # Individual static assets:
  install -Dm644 "$srcdir/clerk-modular/public/index.html" \
                  "$pkgdir/usr/share/clerk-web/index.html"
  install -Dm644 "$srcdir/clerk-modular/public/script.js"  \
                  "$pkgdir/usr/share/clerk-web/script.js"

  # systemd user unit - now sourced from the git repo as well
  install -Dm644 "$srcdir/clerk-modular/clerk-web.service" \
                  "$pkgdir/usr/lib/systemd/user/clerk-web.service"
}
