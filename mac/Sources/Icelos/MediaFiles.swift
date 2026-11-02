import AppKit
import UniformTypeIdentifiers

/// MediaFiles finds the video and audio files a user hands to the app.
enum MediaFiles {
    /// expand returns media files from urls, walking into folders.
    static func expand(_ urls: [URL]) -> [URL] {
        let fm = FileManager.default
        var out: [URL] = []
        for url in urls where url.isFileURL {
            var isDir: ObjCBool = false
            guard fm.fileExists(atPath: url.path, isDirectory: &isDir) else { continue }
            if !isDir.boolValue {
                if isMedia(url) { out.append(url) }
                continue
            }
            let walker = fm.enumerator(at: url, includingPropertiesForKeys: [.isRegularFileKey],
                                       options: [.skipsHiddenFiles, .skipsPackageDescendants])
            while let file = walker?.nextObject() as? URL {
                if isMedia(file) { out.append(file) }
            }
        }
        return out.sorted { $0.path.localizedStandardCompare($1.path) == .orderedAscending }
    }

    /// isMedia reports whether url looks like video or audio.
    static func isMedia(_ url: URL) -> Bool {
        guard let type = UTType(filenameExtension: url.pathExtension) else { return false }
        return type.conforms(to: .audiovisualContent)
    }

    /// choose shows an open panel and passes the chosen media files to done.
    @MainActor
    static func choose(_ done: @escaping ([URL]) -> Void) {
        let panel = NSOpenPanel()
        panel.allowsMultipleSelection = true
        panel.canChooseDirectories = true
        panel.allowedContentTypes = [.audiovisualContent, .folder]
        panel.prompt = "Add"
        if panel.runModal() == .OK { done(expand(panel.urls)) }
    }

    /// chooseFolder shows a folder picker and passes the folder to done.
    @MainActor
    static func chooseFolder(_ done: @escaping (URL) -> Void) {
        let panel = NSOpenPanel()
        panel.canChooseFiles = false
        panel.canChooseDirectories = true
        panel.canCreateDirectories = true
        panel.prompt = "Use Folder"
        if panel.runModal() == .OK, let url = panel.url { done(url) }
    }
}
