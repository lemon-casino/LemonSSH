import { useCallback } from "react";
import { netcattyBridge } from "../../../infrastructure/services/netcattyBridge";
import type { SftpFileEntry, SftpFilenameEncoding } from "../../../domain/models";
import { buildMockLocalFiles } from "./mockLocalFiles";
import { formatFileSize, formatDate } from "./utils";

export const useSftpDirectoryListing = () => {
  const getLocalHomeDir = useCallback(async (): Promise<string> => {
    const bridge = netcattyBridge.get();
    if (!bridge) {
      return navigator.platform.toLowerCase().includes("win") ? "C:\\Users\\damao" : "/Users/damao";
    }
    if (!bridge.getHomeDir) throw new Error("getHomeDir unavailable");
    const homeDir = await bridge.getHomeDir();
    if (!homeDir) throw new Error("Local home directory unavailable");
    return homeDir;
  }, []);

  const listLocalFiles = useCallback(
    async (path: string): Promise<SftpFileEntry[]> => {
      const bridge = netcattyBridge.get();
      if (!bridge) return buildMockLocalFiles(path);
      if (!bridge.listLocalDir) throw new Error("listLocalDir unavailable");
      const rawFiles = await bridge.listLocalDir(path);

      return rawFiles.map((f) => {
        const size = parseInt(f.size) || 0;
        const lastModified = new Date(f.lastModified).getTime();
        return {
          name: f.name,
          type: f.type as "file" | "directory" | "symlink",
          size,
          sizeFormatted: formatFileSize(size),
          lastModified,
          lastModifiedFormatted: formatDate(lastModified),
          linkTarget: f.linkTarget as "file" | "directory" | null | undefined,
          hidden: f.hidden,
          owner: f.owner,
        };
      });
    },
    [],
  );

  const listRemoteFiles = useCallback(
    async (sftpId: string, path: string, encoding?: SftpFilenameEncoding): Promise<SftpFileEntry[]> => {
      const rawFiles = await netcattyBridge.get()?.listSftp(sftpId, path, encoding);
      if (!rawFiles) return [];

      return rawFiles.map((f) => {
        const size = parseInt(f.size) || 0;
        const lastModified = new Date(f.lastModified).getTime();
        return {
          name: f.name,
          type: f.type as "file" | "directory" | "symlink",
          size,
          sizeFormatted: formatFileSize(size),
          lastModified,
          lastModifiedFormatted: formatDate(lastModified),
          permissions: f.permissions,
          owner: f.owner,
          linkTarget: f.linkTarget as "file" | "directory" | null | undefined,
        };
      });
    },
    [],
  );

  return {
    getLocalHomeDir,
    listLocalFiles,
    listRemoteFiles,
  };
};
