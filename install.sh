#!/bin/bash

# Sandbox Installation Script
# This script installs the required tools for the sandbox application

set -e

echo "🚀 Installing required tools for Sandbox..."

# Detect OS
if [[ "$OSTYPE" == "linux-gnu"* ]]; then
    OS="linux"
elif [[ "$OSTYPE" == "darwin"* ]]; then
    OS="macos"
else
    echo "❌ Unsupported OS: $OSTYPE"
    exit 1
fi

echo "📍 Detected OS: $OS"

# Function to check if command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Install Docker
if command_exists docker; then
    echo "✅ Docker is already installed"
else
    echo "📦 Installing Docker..."
    if [[ "$OS" == "macos" ]]; then
        echo "Please install Docker Desktop for Mac from: https://docs.docker.com/desktop/install/mac-install/"
        echo "Or use Homebrew: brew install --cask docker"
        echo "Then run: open /Applications/Docker.app"
    elif [[ "$OS" == "linux" ]]; then
        curl -fsSL https://get.docker.com -o get-docker.sh
        sudo sh get-docker.sh
        sudo usermod -aG docker $USER
        rm get-docker.sh
        echo "Please log out and log back in for Docker group changes to take effect"
    fi
fi

# Install Git
if command_exists git; then
    echo "✅ Git is already installed"
else
    echo "📦 Installing Git..."
    if [[ "$OS" == "macos" ]]; then
        if command_exists brew; then
            brew install git
        else
            echo "Please install Homebrew first: /bin/bash -c \"\$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)\""
            exit 1
        fi
    elif [[ "$OS" == "linux" ]]; then
        if command_exists apt; then
            sudo apt update && sudo apt install -y git
        elif command_exists yum; then
            sudo yum install -y git
        elif command_exists dnf; then
            sudo dnf install -y git
        else
            echo "❌ Unsupported package manager. Please install git manually."
            exit 1
        fi
    fi
fi

# Install Go (optional, for development only)
if [[ "${INSTALL_GO:-false}" == "true" ]]; then
    if command_exists go; then
        echo "✅ Go is already installed"
    else
        echo "📦 Installing Go (development only)..."
        if [[ "$OS" == "macos" ]]; then
            if command_exists brew; then
                brew install go
            else
                echo "Please install Go from: https://golang.org/dl/"
            fi
        elif [[ "$OS" == "linux" ]]; then
            GO_VERSION="1.21.5"
            if [[ "$(uname -m)" == "x86_64" ]]; then
                ARCH="amd64"
            elif [[ "$(uname -m)" == "aarch64" ]]; then
                ARCH="arm64"
            else
                ARCH="amd64"
            fi

            wget "https://golang.org/dl/go${GO_VERSION}.linux-${ARCH}.tar.gz"
            sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf "go${GO_VERSION}.linux-${ARCH}.tar.gz"
            rm "go${GO_VERSION}.linux-${ARCH}.tar.gz"

            # Add to PATH
            if ! grep -q "/usr/local/go/bin" ~/.bashrc; then
                echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
            fi
            export PATH=$PATH:/usr/local/go/bin
        fi
    fi
else
    echo "ℹ️  Go installation skipped (set INSTALL_GO=true to install for development)"
fi

echo "🎉 Installation complete!"
echo ""
echo "Next steps:"
echo "1. If you installed Docker, start the Docker daemon"
echo "2. If you installed Go, restart your terminal or run: source ~/.bashrc"
echo "3. Run: go build -o sandbox cmd/sandbox/main.go"
echo "4. Try: ./sandbox --help"
