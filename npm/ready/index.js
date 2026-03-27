#!/usr/bin/env node
'use strict';

const { execFileSync, execSync } = require('child_process');
const path = require('path');
const fs = require('fs');
const os = require('os');

function getBinaryPath() {
  const platform = os.platform();
  const arch = os.arch();

  // Check if rd is already on PATH
  try {
    const which = platform === 'win32' ? 'where' : 'which';
    const systemBin = execSync(`${which} rd`, { encoding: 'utf8', stdio: 'pipe' }).trim();
    if (systemBin) return systemBin;
  } catch {}

  // Check cache
  const cacheDir = path.join(os.homedir(), '.cache', 'ready');
  const binName = platform === 'win32' ? 'rd.exe' : 'rd';
  const cachedBin = path.join(cacheDir, binName);
  if (fs.existsSync(cachedBin)) return cachedBin;

  // Download from GitHub Releases
  const crypto = require('crypto');
  const goArch = arch === 'x64' ? 'amd64' : arch;
  const ext = platform === 'win32' ? 'zip' : 'tar.gz';
  const archiveName = `rd_${platform === 'win32' ? 'windows' : platform}_${goArch}.${ext}`;
  const baseUrl = 'https://github.com/campfire-net/ready/releases/latest/download';
  const archiveUrl = `${baseUrl}/${archiveName}`;
  const checksumsUrl = `${baseUrl}/checksums.txt`;
  const archivePath = path.join(cacheDir, archiveName);

  process.stderr.write(`rd: downloading ${archiveName}\n`);
  fs.mkdirSync(cacheDir, { recursive: true });

  try {
    execSync(`curl -sL "${archiveUrl}" -o "${archivePath}"`, { stdio: 'pipe' });
    const checksums = execSync(`curl -sL "${checksumsUrl}"`, { encoding: 'utf8' });

    // Verify SHA256
    const archiveData = fs.readFileSync(archivePath);
    const actualHash = crypto.createHash('sha256').update(archiveData).digest('hex');
    const expectedLine = checksums.split('\n').find(l => l.includes(archiveName));
    if (!expectedLine) {
      throw new Error(`checksum not found for ${archiveName}`);
    }
    const expectedHash = expectedLine.trim().split(/\s+/)[0];
    if (actualHash !== expectedHash) {
      fs.unlinkSync(archivePath);
      throw new Error(`checksum mismatch: expected ${expectedHash}, got ${actualHash}`);
    }
    process.stderr.write(`rd: checksum verified\n`);

    // Extract
    if (platform === 'win32') {
      execSync(`cd "${cacheDir}" && tar -xf "${archivePath}" --strip-components=1 --include='*/rd.exe'`, { stdio: 'pipe' });
    } else {
      execSync(`tar xzf "${archivePath}" -C "${cacheDir}" --strip-components=1 --wildcards '*/rd'`, { stdio: 'pipe' });
    }
    fs.unlinkSync(archivePath);
    fs.chmodSync(cachedBin, 0o755);
    if (fs.existsSync(cachedBin)) return cachedBin;
  } catch (e) {
    throw new Error(`rd: failed to download/verify binary: ${e.message}\nInstall manually: curl -fsSL https://ready.getcampfire.dev/install.sh | sh`);
  }

  throw new Error('rd: could not find or download binary');
}

try {
  const bin = getBinaryPath();
  execFileSync(bin, process.argv.slice(2), { stdio: 'inherit' });
} catch (err) {
  if (err.status) process.exit(err.status);
  process.stderr.write(err.message + '\n');
  process.exit(1);
}
