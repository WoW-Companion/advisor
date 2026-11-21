wowc-advisor: quick start
=========================

This folder holds the advisor program, its license and the third-party notices.
It contains no game data; the advisor downloads that on first use.

Windows
  1. Unzip this folder anywhere.
  2. Open a Command Prompt or PowerShell in it and run:
         wowc-advisor.exe doctor
  3. Windows SmartScreen may say it "protected your PC" because the program is not
     signed. Click "More info", then "Run anyway".

macOS
  1. Unpack the archive: tar -xzf <the file you downloaded>
  2. If Gatekeeper blocks the program, run:
         xattr -d com.apple.quarantine wowc-advisor
  3. Then run:  ./wowc-advisor doctor

Linux (for example under Wine or Proton setups)
  1. Unpack the archive: tar -xzf <the file you downloaded>
  2. Run:  ./wowc-advisor doctor

The program is not code-signed or notarized: that needs paid certificates.
Check the download against SHA256SUMS on the releases page if you want to be sure.

Next steps: https://github.com/WoW-Companion/advisor/blob/main/docs/quickstart.md
