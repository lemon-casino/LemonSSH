// Windows version resource metadata (winres format consumed by
// `wails3 generate syso -info`). The packaged version always comes from
// package.json so the executable's file properties match main.version;
// the stale goversioninfo-era cmd/lemonssh/versioninfo.json was retired
// because nothing consumed it.
export function versionParts(version) {
  const match = String(version ?? "")
    .trim()
    .match(/^v?(\d+)(?:\.(\d+))?(?:\.(\d+))?(?:[.+_-].*)?$/);
  if (!match) throw new Error(`unsupported version string: ${version}`);
  return {
    major: Number(match[1]),
    minor: Number(match[2] ?? 0),
    patch: Number(match[3] ?? 0),
    build: 0,
  };
}

// winresVersionInfo renders the winres version.Info JSON: fixed numeric
// versions for the file-properties dialog plus a language string table
// (0409 = en-US, matching the previous versioninfo.json translation).
export function winresVersionInfo({
  version,
  productName,
  companyName,
  fileDescription,
  internalName,
  originalFilename,
  legalCopyright = "",
  comments = "",
  langID = "0409",
}) {
  if (!version) throw new Error("version is required");
  const { major, minor, patch, build } = versionParts(version);
  const fixedVersion = `${major}.${minor}.${patch}.${build}`;
  return {
    fixed: {
      file_version: fixedVersion,
      product_version: fixedVersion,
    },
    info: {
      [langID]: {
        Comments: comments,
        CompanyName: companyName,
        FileDescription: fileDescription,
        FileVersion: version,
        InternalName: internalName,
        LegalCopyright: legalCopyright,
        OriginalFilename: originalFilename,
        ProductName: productName,
        ProductVersion: version,
      },
    },
  };
}

// lemonsshWinresInfo carries the branding constants that used to live in
// cmd/lemonssh/versioninfo.json.
export function lemonsshWinresInfo(version) {
  return winresVersionInfo({
    version,
    productName: "LemonSSH",
    companyName: "LemonSSH",
    fileDescription: "LemonSSH",
    internalName: "LemonSSH",
    originalFilename: "LemonSSH.exe",
  });
}
