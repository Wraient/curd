# Maintainer: Wraient <rushikeshwastaken@gmail.com>
pkgname='curd'
pkgver='r86.2ccc8b0'
pkgrel=1
pkgdesc="Watch anime in cli with Anilist Tracking, Discord RPC, Intro Outro Skipping, etc."
arch=("x86_64")
url="https://github.com/Wraient/curd"
license=('GPL')
depends=('python' 'mpv' 'socat')
makedepends=('python-pip')
source=("https://raw.githubusercontent.com/Wraient/curd/refs/heads/main/curd.py")
sha256sums=('SKIP')

package() {
    # Create the directory for the virtual environment
    install -dm755 "$pkgdir/usr/share/curd"

    # Create a virtual environment
    python -m venv "$pkgdir/usr/share/curd/venv"

    # Install pypresence in the virtual environment
    "$pkgdir/usr/share/curd/venv/bin/pip" install pypresence requests

    # Install the Python script
    install -Dm755 "$srcdir/curd.py" "$pkgdir/usr/bin/curd.py"

    # Create the run script
    cat << 'EOF' > "$pkgdir/usr/bin/curd"
#!/bin/bash

# Path to the virtual environment
VENV_DIR="/usr/share/curd/venv"

# Activate the virtual environment
source "$VENV_DIR/bin/activate"

# Run the Python script
python /usr/bin/curd.py

# Deactivate the virtual environment after running
deactivate
EOF

    # Make the run script executable
    chmod +x "$pkgdir/usr/bin/curd"
}
