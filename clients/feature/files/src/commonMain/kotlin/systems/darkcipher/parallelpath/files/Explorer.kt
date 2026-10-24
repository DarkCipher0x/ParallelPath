package systems.darkcipher.parallelpath.files

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import systems.darkcipher.parallelpath.model.Attachment
import systems.darkcipher.parallelpath.model.Doc
import systems.darkcipher.parallelpath.model.SharedFile

// The explorer's model: where you are, what's listed, how it's sorted, and the clipboard.

/** A place in the explorer: a folder of the documents, or one of the special views. */
internal sealed interface Loc {
    data class Folder(val path: String) : Loc
    data object Recent : Loc
    data object Shared : Loc
    data object Removed : Loc
}

internal fun Loc.title(): String = when (this) {
    is Loc.Folder -> if (path.isEmpty()) "Documents" else path.substringAfterLast('/')
    Loc.Recent -> "Recent"
    Loc.Shared -> "Shared in messages"
    Loc.Removed -> "Removed"
}

internal enum class ViewMode { Details, Icons }

internal enum class SortBy(val label: String) { Name("Name"), Modified("Date modified"), Type("Type"), Size("Size"), Author("Added by") }

/** One row: a document (file or folder) or a file shared in a message. */
internal class Entry(val doc: Doc? = null, val shared: SharedFile? = null) {
    val key: String = when {
        doc != null -> doc.id.ifEmpty { "dir:" + doc.path }
        else -> "msg:${shared!!.space}/${shared.message}/${shared.file.blobId}"
    }
    val name: String get() = doc?.name ?: shared!!.file.name
    val isFolder: Boolean get() = doc?.isFolder == true
    val file: Attachment? get() = doc?.file ?: shared?.file
    val size: Long get() = file?.size ?: 0
    val modified: String get() = doc?.updated?.ifEmpty { doc.created } ?: shared!!.time
    val author: String get() = when {
        doc != null -> if (doc.implied) "" else if (doc.mine) "You" else doc.authorName
        else -> if (shared!!.mine) "You" else shared.authorName
    }
    val mine: Boolean get() = doc?.mine ?: shared!!.mine
    val removed: Boolean get() = doc?.removed == true
    val where: String get() = when {
        doc != null -> if (doc.folder.isEmpty()) "Documents" else "Documents/${doc.folder}"
        shared!!.conversation.isNotEmpty() -> "Private: ${shared.conversation}"
        else -> "Family messages"
    }
    val type: String get() = if (isFolder) "Folder" else typeName(name, file?.mime.orEmpty())
}

internal fun typeName(name: String, mime: String): String {
    val ext = name.substringAfterLast('.', "").lowercase()
    return when {
        mime == "application/pdf" || ext == "pdf" -> "PDF document"
        mime.startsWith("image/") -> (if (ext.isNotEmpty()) ext.uppercase() + " " else "") + "image"
        mime.startsWith("video/") -> (if (ext.isNotEmpty()) ext.uppercase() + " " else "") + "video"
        mime.startsWith("audio/") -> "Audio"
        ext in setOf("doc", "docx", "odt", "rtf") -> "Word document"
        ext in setOf("xls", "xlsx", "ods", "csv") -> "Spreadsheet"
        ext in setOf("ppt", "pptx", "odp") -> "Presentation"
        ext in setOf("txt", "md", "log") -> "Text document"
        ext in setOf("zip", "7z", "rar", "gz") -> "Compressed archive"
        ext.isNotEmpty() -> ext.uppercase() + " file"
        else -> "File"
    }
}

internal fun sortEntries(list: List<Entry>, by: SortBy, descending: Boolean): List<Entry> {
    val c: Comparator<Entry> = when (by) {
        SortBy.Name -> compareBy(String.CASE_INSENSITIVE_ORDER) { it.name }
        SortBy.Modified -> compareBy { it.modified }
        SortBy.Type -> compareBy<Entry, String>(String.CASE_INSENSITIVE_ORDER) { it.type }.thenBy(String.CASE_INSENSITIVE_ORDER) { it.name }
        SortBy.Size -> compareBy<Entry> { it.size }.thenBy(String.CASE_INSENSITIVE_ORDER) { it.name }
        SortBy.Author -> compareBy<Entry, String>(String.CASE_INSENSITIVE_ORDER) { it.author }.thenBy(String.CASE_INSENSITIVE_ORDER) { it.name }
    }
    val sorted = list.sortedWith(if (descending) c.reversed() else c)
    return sorted.filter { it.isFolder } + sorted.filterNot { it.isFolder } // folders first, as in every file manager
}

/** Cut or copied documents, kept while you move around (even to another family). */
internal data class Clip(val cut: Boolean, val space: String, val docs: List<Doc>)

internal object ExplorerClipboard {
    var clip by mutableStateOf<Clip?>(null)
}

/** The prefs that stay while the app is open. */
internal object ExplorerPrefs {
    var view by mutableStateOf(ViewMode.Details)
    var sortBy by mutableStateOf(SortBy.Name)
    var descending by mutableStateOf(false)
    var detailsPane by mutableStateOf(true)
    var tree by mutableStateOf(true)
}

/** The folders of the documents, as a tree (implied ones included). */
internal fun folderPaths(all: List<Doc>): List<String> =
    all.filter { it.isFolder && !it.removed }.map { it.path }.distinct().sortedWith(String.CASE_INSENSITIVE_ORDER)

internal fun childFolders(paths: List<String>, parent: String): List<String> =
    paths.filter { (if (parent.isEmpty()) !it.contains('/') else it.startsWith("$parent/") && !it.removePrefix("$parent/").contains('/')) }
