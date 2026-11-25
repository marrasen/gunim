package gunim.android;

import android.content.ContentProviderClient;
import android.content.ContentResolver;
import android.content.Context;
import android.content.Intent;
import android.database.Cursor;
import android.net.Uri;
import android.os.Build;
import android.os.ParcelFileDescriptor;
import android.provider.DocumentsContract;
import android.provider.DocumentsContract.Document;
import java.io.IOException;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.atomic.AtomicLong;

/**
 * GunimSave writes the files and the folders the user chose a place for.
 * Their documents are another application's, and each call here may wait
 * for it. Go calls them from goroutines of its own, and no error of that
 * application's goes further than a failure.
 */
final class GunimSave {
	private static final int WRITE = Intent.FLAG_GRANT_READ_URI_PERMISSION | Intent.FLAG_GRANT_WRITE_URI_PERMISSION;

	/** Open is a document being written, and the provider it is kept by. */
	private static final class Open {
		final ParcelFileDescriptor file;
		final ContentProviderClient by;

		Open(ParcelFileDescriptor file, ContentProviderClient by) {
			this.file = file;
			this.by = by;
		}
	}

	private static final ConcurrentHashMap<Long, Open> open = new ConcurrentHashMap<>();
	private static final AtomicLong last = new AtomicLong();

	private GunimSave() {
	}

	/** Doing is a call to a provider. */
	private interface Doing<T> {
		T with(ContentResolver r, ContentProviderClient by) throws Exception;
	}

	/**
	 * with calls the provider of u, held unstably where Android can: one
	 * that ends in the call then ends the call, and not this program.
	 */
	private static <T> T with(Uri u, T failed, Doing<T> what) {
		ContentProviderClient by = null;
		try {
			ContentResolver r = Native.app.getContentResolver();
			by = r.acquireUnstableContentProviderClient(u);
			if (by == null) {
				return failed;
			}
			T v = what.with(Build.VERSION.SDK_INT >= 29 ? ContentResolver.wrap(by) : r, by);
			return v == null ? failed : v;
		} catch (Exception | OutOfMemoryError | StackOverflowError e) {
			return failed;
		} finally {
			if (by != null) {
				by.close();
			}
		}
	}

	/** column returns a column of the document at u, or null. */
	private static String column(ContentResolver r, Uri u, String column) {
		try (Cursor c = r.query(u, new String[] {column}, null, null, null)) {
			return c != null && c.moveToFirst() && !c.isNull(0) ? c.getString(0) : null;
		} catch (RuntimeException e) {
			return null;
		}
	}

	private static String nameOf(ContentResolver r, Uri u, String otherwise) {
		String name = column(r, u, Document.COLUMN_DISPLAY_NAME);
		return name == null || name.length() > 1000 ? otherwise : name.replace('\0', '_');
	}

	/**
	 * take keeps the right to write what the user chose, so that a later
	 * run can take away a file this one leaves half written.
	 */
	static void take(Uri u) {
		try {
			Native.app.getContentResolver().takePersistableUriPermission(u, WRITE);
		} catch (RuntimeException e) {
			// It lasts as long as the process.
		}
	}

	/** letGo gives the right back. */
	static void letGo(String place) {
		try {
			Uri u = Uri.parse(place);
			if (DocumentsContract.isTreeUri(u)) {
				// A folder's file is kept by the right to the folder.
				u = DocumentsContract.buildTreeDocumentUri(u.getAuthority(), DocumentsContract.getTreeDocumentId(u));
			}
			Native.app.getContentResolver().releasePersistableUriPermission(u, WRITE);
		} catch (RuntimeException e) {
			// There was none.
		}
	}

	/**
	 * about returns the name of the document at u, and, after a NUL, 1
	 * where the choice made it, as empty and no older than when the user
	 * was asked, and 0 where it was there, or does not tell.
	 */
	static String about(Uri u, long asked, String otherwise) {
		return with(u, otherwise + "\0" + 0, (r, by) -> {
			String size = column(r, u, Document.COLUMN_SIZE), at = column(r, u, Document.COLUMN_LAST_MODIFIED);
			boolean made = "0".equals(size) && (at == null || Long.parseLong(at) >= asked - 2000);
			return nameOf(r, u, otherwise) + "\0" + (made ? 1 : 0);
		});
	}

	/**
	 * stray takes away the empty file a picker made for a process that
	 * ended before its answer came.
	 */
	static void stray(Uri u, long asked) {
		if (!DocumentsContract.isTreeUri(u) && about(u, asked, "").endsWith("1")) {
			delete(u.toString());
		}
	}

	/**
	 * open opens the document at place to write from its start, and
	 * returns a descriptor for Go to write and close, and the id it is kept
	 * by, as "fd id", or "".
	 */
	static String open(String place) {
		Uri u = Uri.parse(place);
		ContentProviderClient by = null;
		ParcelFileDescriptor file = null;
		try {
			by = Native.app.getContentResolver().acquireUnstableContentProviderClient(u);
			if (by == null) {
				return "";
			}
			// What was there goes: "w" alone leaves the end of a longer one.
			for (String mode : new String[] {"wt", "rwt"}) {
				try {
					file = by.openFile(u, mode);
				} catch (IOException | RuntimeException e) {
					continue;
				}
				if (file != null) {
					break;
				}
			}
			if (file == null) {
				return "";
			}
			int fd = ParcelFileDescriptor.dup(file.getFileDescriptor()).detachFd();
			long id = last.incrementAndGet();
			open.put(id, new Open(file, by));
			file = null;
			by = null;
			return fd + " " + id;
		} catch (Exception | OutOfMemoryError | StackOverflowError e) {
			return "";
		} finally {
			abandon(file);
			if (by != null) {
				by.close();
			}
		}
	}

	/** finish closes the document kept as id as whole, and reports whether whoever keeps it has not given up. */
	static boolean finish(long id) {
		Open o = open.remove(id);
		if (o == null) {
			return false;
		}
		try {
			if (o.file.canDetectErrors()) {
				o.file.checkError();
			}
			o.file.close();
			return true;
		} catch (ParcelFileDescriptor.FileDescriptorDetachedException e) {
			// It handed its end on, where Android loses sight of it.
			return true;
		} catch (IOException | RuntimeException e) {
			abandon(o.file);
			return false;
		} finally {
			o.by.close();
		}
	}

	/** abandon closes the document kept as id, and says so to whoever keeps it. */
	static void abandon(long id) {
		Open o = open.remove(id);
		if (o != null) {
			abandon(o.file);
			o.by.close();
		}
	}

	private static void abandon(ParcelFileDescriptor file) {
		if (file == null) {
			return;
		}
		try {
			file.closeWithError("not whole");
		} catch (IOException | RuntimeException e) {
			// It is let go of all the same.
		}
	}

	/** delete takes the document at place away: 1 where it is gone, and 0 where it is not. */
	static boolean delete(String place) {
		Uri u = Uri.parse(place);
		return with(u, false, (r, by) -> {
			try {
				if (DocumentsContract.deleteDocument(r, u)) {
					return true;
				}
			} catch (Exception e) {
				// It may be gone.
			}
			return column(r, u, Document.COLUMN_DOCUMENT_ID) == null && !isDenied(r, u);
		});
	}

	// isDenied reports whether this program may not ask of u, which then
	// may be there.
	private static boolean isDenied(ContentResolver r, Uri u) {
		return Native.app.checkUriPermission(u, android.os.Process.myPid(), android.os.Process.myUid(), WRITE)
			!= android.content.pm.PackageManager.PERMISSION_GRANTED;
	}

	// A folder's documents, by the address of the folder the user chose and
	// the id of the folder in it.

	/** rootOf returns the id of the folder the user chose. */
	static String rootOf(String tree) {
		try {
			return DocumentsContract.getTreeDocumentId(Uri.parse(tree));
		} catch (RuntimeException e) {
			return "";
		}
	}

	/**
	 * find looks for name in the folder in, whatever its case, as some
	 * disks tell none apart: its id after "d", for a folder, or "f", "" for
	 * none, or "!" where it can't tell.
	 */
	static String find(String tree, String in, String name) {
		Uri u = DocumentsContract.buildChildDocumentsUriUsingTree(Uri.parse(tree), in);
		return with(u, "!", (r, by) -> {
			try (Cursor c = r.query(u, new String[] {Document.COLUMN_DOCUMENT_ID, Document.COLUMN_DISPLAY_NAME,
				Document.COLUMN_MIME_TYPE}, null, null, null)) {
				if (c == null) {
					return "!";
				}
				while (c.moveToNext()) {
					if (name.equalsIgnoreCase(c.getString(1))) {
						return (Document.MIME_TYPE_DIR.equals(c.getString(2)) ? "d" : "f") + c.getString(0);
					}
				}
				return "";
			}
		});
	}

	/** mkdir makes a folder in the folder in, and returns its id, or "". */
	static String mkdir(String tree, String in, String name) {
		Uri parent = DocumentsContract.buildDocumentUriUsingTree(Uri.parse(tree), in);
		return with(parent, "", (r, by) -> {
			Uri u = DocumentsContract.createDocument(r, parent, Document.MIME_TYPE_DIR, name);
			if (u == null) {
				return "";
			}
			if (!name.equals(nameOf(r, u, name))) {
				// It made one of another name.
				DocumentsContract.deleteDocument(r, u);
				return "";
			}
			return DocumentsContract.getDocumentId(u);
		});
	}

	/**
	 * create makes a file of type in the folder in. Its name is part until
	 * rename, where the folder lets a file be renamed, and name otherwise.
	 * It returns the file's address and, after a NUL, the name, or "".
	 */
	static String create(String tree, String in, String name, String part, String type) {
		Uri parent = DocumentsContract.buildDocumentUriUsingTree(Uri.parse(tree), in);
		return with(parent, "", (r, by) -> {
			Uri u = DocumentsContract.createDocument(r, parent, "application/octet-stream", part);
			if (u == null) {
				return "";
			}
			String flags = column(r, u, Document.COLUMN_FLAGS);
			if (flags == null || (Integer.parseInt(flags) & Document.FLAG_SUPPORTS_RENAME) == 0) {
				DocumentsContract.deleteDocument(r, u);
				u = DocumentsContract.createDocument(r, parent, type, name);
				if (u == null) {
					return "";
				}
			}
			return u + "\0" + nameOf(r, u, part);
		});
	}

	/** rename names the document at place, and returns its address and, after a NUL, its name, or "". */
	static String rename(String place, String name) {
		Uri u = Uri.parse(place);
		return with(u, "", (r, by) -> {
			Uri to = DocumentsContract.renameDocument(r, u, name);
			Uri now = to == null ? u : to;
			return now + "\0" + nameOf(r, now, name);
		});
	}
}
