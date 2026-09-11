import type { RemoteFile } from "../../../domain/models/workspace";

type TreeOptions = Parameters<NonNullable<NetcattyBridge["listLocalTree"]>>[1];
type TreeEntry = Awaited<ReturnType<NonNullable<NetcattyBridge["listLocalTree"]>>>[number];

// Reuse native directory reads; no file contents cross the renderer during scanning.
export async function readLocalTree(
  root: string,
  listDirectory: (path: string) => Promise<RemoteFile[]>,
  options: TreeOptions = {},
  signal?: AbortSignal,
): Promise<TreeEntry[]> {
  const trimmedRoot = root.replace(/[\\/]+$/, "");
  const rootName = trimmedRoot.split(/[\\/]/).pop() || "folder";
  const separator = root.includes("\\") ? "\\" : "/";
  const join = (base: string, name: string) => base.endsWith(separator) ? base + name : base + separator + name;
  const results: TreeEntry[] = [{ localPath: root, relativePath: rootName, type: "directory", size: 0, lastModified: 0 }];
  const pending = [results[0]];
  const maxEntries = options.limits?.maxEntries ?? 200_000;
  const maxDirectories = options.limits?.maxDirectories ?? 50_000;
  let directories = 1;
  let files = 0;
  const check = () => {
    signal?.throwIfAborted();
    if (results.length > maxEntries || directories > maxDirectories) {
      throw new Error("Local directory traversal limit exceeded. Select a smaller folder to upload.");
    }
  };
  check();
  options.onEntries?.([results[0]]);
  while (pending.length) {
    check();
    const parent = pending.pop()!;
    const entries = await listDirectory(parent.localPath);
    check();
    const batch: TreeEntry[] = [];
    for (const entry of entries) {
      // Do not follow directory symlinks into cycles or outside the dropped tree.
      if (entry.type === "symlink" && entry.linkTarget !== "file") continue;
      const row: TreeEntry = {
        localPath: join(parent.localPath, entry.name),
        relativePath: `${parent.relativePath}/${entry.name}`,
        type: entry.type === "directory" ? "directory" : "file",
        size: entry.type === "directory" ? 0 : Number.parseInt(entry.size, 10) || 0,
        lastModified: Date.parse(entry.lastModified) || 0,
      };
      results.push(row);
      batch.push(row);
      if (row.type === "directory") { directories++; pending.push(row); }
      else files++;
      check();
    }
    options.onEntries?.(batch);
    options.onProgress?.({ fileCount: files, directoryCount: directories, entryCount: results.length });
  }
  check();
  return results;
}
