export interface DroppedFile {
  file: File
  filepath: string
}

export function entryToFile(entry: FileSystemFileEntry): Promise<File | null> {
  return new Promise(resolve => {
    entry.file(f => resolve(f), () => resolve(null))
  })
}

export async function readDirectoryEntry(
  dirEntry: FileSystemDirectoryEntry,
  basePath: string,
  out: DroppedFile[],
): Promise<void> {
  const reader = dirEntry.createReader()
  let entries: FileSystemEntry[] = []
  let batch: FileSystemEntry[]
  do {
    batch = await new Promise<FileSystemEntry[]>((resolve, reject) => {
      reader.readEntries(resolve, reject)
    })
    entries = entries.concat(batch)
  } while (batch.length > 0)

  for (const entry of entries) {
    const path = basePath ? basePath + '/' + entry.name : entry.name
    if (entry.isFile) {
      const file = await entryToFile(entry as FileSystemFileEntry)
      if (file) out.push({ file, filepath: path })
    } else if (entry.isDirectory) {
      await readDirectoryEntry(entry as FileSystemDirectoryEntry, path, out)
    }
  }
}

/**
 * Parse a DataTransfer from a drop event into single files and folder contents.
 */
export async function parseDropItems(
  dataTransfer: DataTransfer,
): Promise<{ files: File[]; folderFiles: DroppedFile[] }> {
  const files: File[] = []
  const folderFiles: DroppedFile[] = []

  const items = dataTransfer.items
  if (!items?.length) return { files, folderFiles }

  const entries: FileSystemEntry[] = []
  for (const item of items) {
    const entry = item.webkitGetAsEntry?.()
    if (entry) entries.push(entry)
  }

  for (const entry of entries) {
    if (entry.isFile) {
      const file = await entryToFile(entry as FileSystemFileEntry)
      if (file) files.push(file)
    } else if (entry.isDirectory) {
      await readDirectoryEntry(entry as FileSystemDirectoryEntry, entry.name, folderFiles)
    }
  }

  return { files, folderFiles }
}
