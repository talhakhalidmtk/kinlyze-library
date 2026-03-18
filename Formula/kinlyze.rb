# typed: false
# frozen_string_literal: true

# Homebrew formula for Kinlyze.
# This file lives in: github.com/kinlyze/homebrew-tap
# Automatically updated by GoReleaser on each release.

class Kinlyze < Formula
  desc     "Analyze the kin behind your code — map knowledge concentration risk"
  homepage "https://kinlyze.com"
  version  "0.1.0"
  license  "MIT"

  on_macos do
    on_intel do
      url      "https://github.com/talhakhalidmtk/kinlyze-library/releases/download/v#{version}/kinlyze_#{version}_darwin_amd64.tar.gz"
      sha256   "REPLACE_WITH_ACTUAL_SHA256_FROM_GORELEASER"
    end
    on_arm do
      url      "https://github.com/talhakhalidmtk/kinlyze-library/releases/download/v#{version}/kinlyze_#{version}_darwin_arm64.tar.gz"
      sha256   "REPLACE_WITH_ACTUAL_SHA256_FROM_GORELEASER"
    end
  end

  on_linux do
    on_intel do
      url      "https://github.com/talhakhalidmtk/kinlyze-library/releases/download/v#{version}/kinlyze_#{version}_linux_amd64.tar.gz"
      sha256   "REPLACE_WITH_ACTUAL_SHA256_FROM_GORELEASER"
    end
    on_arm do
      url      "https://github.com/talhakhalidmtk/kinlyze-library/releases/download/v#{version}/kinlyze_#{version}_linux_arm64.tar.gz"
      sha256   "REPLACE_WITH_ACTUAL_SHA256_FROM_GORELEASER"
    end
  end

  def install
    bin.install "kinlyze"
  end

  test do
    system "#{bin}/kinlyze", "version"
  end
end
