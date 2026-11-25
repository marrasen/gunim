package gunim.android;

import android.content.ContentProviderClient;
import android.content.ContentResolver;
import android.content.Context;
import android.content.Intent;
import android.content.UriPermission;
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
	static <T> T with(Uri u, T failed, Doing<T> what) {
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
	private static void take(Uri u) {
		try {
			Native.app.getContentResolver().takePersistableUriPermission(u, WRITE);
		} catch (RuntimeException e) {
			// It lasts as long as the process.
		}
	}

	/** rightTo returns the address of what the right to u is kept by: a folder's file by the folder. */
	private static Uri rightTo(Uri u) {
		try {
			if (DocumentsContract.isTreeUri(u)) {
				return DocumentsContract.buildTreeDocumentUri(u.getAuthority(), DocumentsContract.getTreeDocumentId(u));
			}
		} catch (RuntimeException e) {
			// It is no such address.
		}
		return u;
	}

	/** isKept reports whether a right to u is kept, from this run or an earlier one. */
	private static boolean isKept(Uri u) {
		Uri right = rightTo(u);
		for (UriPermission p : Native.app.getContentResolver().getPersistedUriPermissions()) {
			if (p.getUri().equals(right)) {
				return true;
			}
		}
		return false;
	}

	// inUse counts the files and folders of this run that use a right, and
	// earlier are those of the rights that an earlier run kept. They stay:
	// the program may need them to take away what that left.
	private static final java.util.HashMap<Uri, Integer> inUse = new java.util.HashMap<>();
	private static final java.util.HashSet<Uri> earlier = new java.util.HashSet<>();

	/** use keeps the right to what the user chose, for one file or folder of this run. */
	static synchronized void use(Uri u) {
		Uri right = rightTo(u);
		Integer n = inUse.get(right);
		if (n == null && isKept(u)) {
			earlier.add(right);
		}
		inUse.put(right, n == null ? 1 : n + 1);
		take(u);
	}

	/** letGo gives the right back, for one file or folder of this run, with the last that uses it. */
	static synchronized void letGo(String place) {
		Uri right = rightTo(Uri.parse(place));
		Integer n = inUse.get(right);
		if (n != null && n > 1) {
			inUse.put(right, n - 1);
			return;
		}
		inUse.remove(right);
		if (n != null && !earlier.contains(right)) {
			release(right);
		}
	}

	private static void release(Uri right) {
		try {
			Native.app.getContentResolver().releasePersistableUriPermission(right, WRITE);
		} catch (RuntimeException e) {
			// There was none.
		}
	}

	/** forget gives back the rights kept by earlier runs, but those to the places in but, each ended by a NUL. */
	static synchronized void forget(String but) {
		java.util.HashSet<Uri> keep = new java.util.HashSet<>(inUse.keySet());
		for (String place : but.split("\0")) {
			if (!place.isEmpty()) {
				keep.add(rightTo(Uri.parse(place)));
			}
		}
		for (UriPermission p : Native.app.getContentResolver().getPersistedUriPermissions()) {
			if (!keep.contains(p.getUri())) {
				release(p.getUri());
				earlier.remove(p.getUri());
			}
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

	// How a document that was closed as whole ended: it is kept, it failed,
	// or nothing tells, as for one that is handed over through a pipe.
	private static final String KEPT = "1", FAILED = "", MAYBE = "?";

	// WAIT is how long finish waits for a word of whoever keeps a document
	// that it has failed, in milliseconds.
	private static final int WAIT = 3000;

	/**
	 * finish closes the document kept as id as whole, and says how it
	 * ended. What is handed over through a pipe is another application's to
	 * keep, after this has closed it, and a word that it took all of it
	 * says nothing of that: it may store it, or send it on, later. So that
	 * one may not be kept, unless a word says that it failed.
	 */
	static String finish(long id, boolean piped) {
		Open o = open.remove(id);
		if (o == null) {
			return FAILED;
		}
		try {
			if (piped) {
				return GunimStreams.closeAndHear(o.file, WAIT) ? FAILED : MAYBE;
			}
			o.file.close();
			return KEPT;
		} catch (IOException | RuntimeException e) {
			return FAILED;
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
			boolean said = false;
			try {
				said = DocumentsContract.deleteDocument(r, u);
			} catch (Exception e) {
				// It may be gone.
			}
			// It says so of a provider that died in the call too, so the
			// document is asked for. One that has died throws here.
			try (Cursor c = by.query(u, new String[] {Document.COLUMN_DOCUMENT_ID}, null, null, null)) {
				return (c == null || c.getCount() == 0) && (said || !isDenied(r, u));
			} catch (RuntimeException e) {
				// There is none to ask of, or the right to it went with it.
				return said;
			}
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
			// A folder in a cloud may tell of some files, and load the rest.
			for (int i = 0; i < LOADS; i++) {
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
					android.os.Bundle more = c.getExtras();
					if (more != null && more.getString(DocumentsContract.EXTRA_ERROR) != null) {
						return "!";
					}
					if (more == null || !more.getBoolean(DocumentsContract.EXTRA_LOADING)) {
						return "";
					}
				}
				Thread.sleep(500);
			}
			return "!";
		});
	}

	// LOADS is how often find looks in a folder that is being loaded, half
	// a second apart.
	private static final int LOADS = 20;

	/** mkdir makes a folder in the folder in, and returns its id, or "". */
	static String mkdir(String tree, String in, String name) {
		Uri parent = DocumentsContract.buildDocumentUriUsingTree(Uri.parse(tree), in);
		return with(parent, "", (r, by) -> {
			Uri u = DocumentsContract.createDocument(r, parent, Document.MIME_TYPE_DIR, name);
			if (u == null) {
				return "";
			}
			if (!name.equals(nameOf(r, u, name))) {
				// It is one of another name. That stays: it may be one that
				// was there.
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
				String size = column(r, u, Document.COLUMN_SIZE);
				if (size != null && !"0".equals(size)) {
					// It is one that was there.
					return "";
				}
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
