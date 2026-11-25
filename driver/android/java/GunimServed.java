package gunim.android;

import android.content.ContentProvider;
import android.content.ContentValues;
import android.content.Context;
import android.database.Cursor;
import android.database.MatrixCursor;
import android.net.Uri;
import android.os.Handler;
import android.os.HandlerThread;
import android.os.ParcelFileDescriptor;
import android.os.ProxyFileDescriptorCallback;
import android.os.storage.StorageManager;
import android.provider.OpenableColumns;
import android.system.ErrnoException;
import android.system.OsConstants;
import java.io.FileNotFoundException;
import java.io.IOException;
import java.security.SecureRandom;
import java.util.List;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.atomic.AtomicInteger;

/**
 * GunimServed hands other applications files that Go reads for them, with
 * no copy on the disk. A file's address holds a word that no other
 * application can guess, and the provider is private: an application reads
 * only what the program granted it. It runs in the program's own process,
 * where the bytes are, so what it serves ends with that.
 */
public final class GunimServed extends ContentProvider {
	/** Item is a file served, which Go knows by its id. */
	private static final class Item {
		final long id, size;
		final String name, type;

		Item(long id, String name, String type, long size) {
			this.id = id;
			this.name = name;
			this.type = type;
			this.size = size;
		}
	}

	private static final ConcurrentHashMap<String, Item> items = new ConcurrentHashMap<>();
	private static final ConcurrentHashMap<Long, Uri> addresses = new ConcurrentHashMap<>();
	private static final SecureRandom random = new SecureRandom();

	// held counts the descriptors other applications have, and MOST is how
	// many they may: each takes a thread.
	private static final AtomicInteger held = new AtomicInteger();
	private static final int MOST = 64;

	// ended says that no file is served from here on.
	private static volatile boolean ended;

	/** uriOf returns the address of the file Go knows as id, the same each time. */
	static synchronized Uri uriOf(Context c, long id, String name, String type, long size) {
		Uri u = addresses.get(id);
		if (u == null) {
			byte[] b = new byte[16];
			random.nextBytes(b);
			StringBuilder word = new StringBuilder();
			for (byte x : b) {
				word.append(String.format("%02x", x));
			}
			items.put(word.toString(), new Item(id, name, type.isEmpty() ? GunimFiles.typeOf(name) : type, size));
			u = new Uri.Builder().scheme("content").authority(c.getPackageName() + ".gunim.served")
				.appendPath(word.toString()).appendPath(name).build();
			addresses.put(id, u);
		}
		return u;
	}

	/** typeOf returns the type of the file at u. */
	static String typeOf(Uri u) {
		Item i = itemOf(u);
		return i == null ? "application/octet-stream" : i.type;
	}

	/** stop ends the serving of the file Go knows as id. */
	static synchronized void stop(long id) {
		Uri u = addresses.remove(id);
		if (u != null) {
			items.remove(u.getPathSegments().get(0));
		}
	}

	/** isOpen reports whether another application has a file open, served still or not. */
	static boolean isOpen() {
		return held.get() > 0;
	}

	/** end ends the serving of every file: a read of one fails, one that waits included. */
	static void end() {
		if (!ended) {
			ended = true;
			Native.servedEnd();
		}
	}

	private static Item itemOf(Uri u) {
		List<String> p = u.getPathSegments();
		return p.isEmpty() || ended ? null : items.get(p.get(0));
	}

	@Override
	public boolean onCreate() {
		return true;
	}

	@Override
	public String getType(Uri u) {
		return typeOf(u);
	}

	@Override
	public ParcelFileDescriptor openFile(Uri u, String mode) throws FileNotFoundException {
		if (!"r".equals(mode)) {
			throw new SecurityException("gunim serves files to read: mode " + mode);
		}
		Item i = itemOf(u);
		if (i == null) {
			// Another run of the program served it.
			throw new FileNotFoundException("no longer served");
		}
		if (held.incrementAndGet() > MOST) {
			held.decrementAndGet();
			throw new FileNotFoundException("too many are open");
		}
		// A read may take long, and each file has a thread for its own.
		HandlerThread t = new HandlerThread("gunim.served");
		t.start();
		Reads r = new Reads(i, t);
		try {
			ParcelFileDescriptor p = getContext().getSystemService(StorageManager.class).openProxyFileDescriptor(
				ParcelFileDescriptor.MODE_READ_ONLY, r, new Handler(t.getLooper()));
			r.open();
			return p;
		} catch (IOException | RuntimeException e) {
			held.decrementAndGet();
			t.quit();
			throw new FileNotFoundException("it could not be opened");
		}
	}

	/** Reads hands the reads of one descriptor to Go. */
	private static final class Reads extends ProxyFileDescriptorCallback {
		private final Item item;
		private final HandlerThread thread;

		Reads(Item item, HandlerThread thread) {
			this.item = item;
			this.thread = thread;
		}

		void open() {
			Native.servedUse(item.id, 1);
		}

		@Override
		public long onGetSize() {
			return item.size;
		}

		@Override
		public int onRead(long offset, int size, byte[] data) throws ErrnoException {
			int n = ended ? -1 : Native.servedRead(item.id, offset, size, data);
			if (n < 0) {
				throw new ErrnoException("read", OsConstants.EIO);
			}
			return n;
		}

		@Override
		public void onFsync() {
			// There is nothing to keep.
		}

		@Override
		public void onRelease() {
			held.decrementAndGet();
			Native.servedUse(item.id, -1);
			thread.quitSafely();
		}
	}

	@Override
	public Cursor query(Uri u, String[] projection, String selection, String[] args, String order) {
		Item i = itemOf(u);
		if (i == null) {
			return null;
		}
		if (projection == null) {
			projection = new String[] {OpenableColumns.DISPLAY_NAME, OpenableColumns.SIZE};
		}
		Object[] row = new Object[projection.length];
		for (int n = 0; n < row.length; n++) {
			if (OpenableColumns.DISPLAY_NAME.equals(projection[n])) {
				row[n] = i.name;
			} else if (OpenableColumns.SIZE.equals(projection[n])) {
				row[n] = i.size;
			}
		}
		MatrixCursor c = new MatrixCursor(projection, 1);
		c.addRow(row);
		return c;
	}

	// The files are the program's own, to read.

	@Override
	public Uri insert(Uri u, ContentValues values) {
		throw new UnsupportedOperationException("gunim serves files to read");
	}

	@Override
	public int delete(Uri u, String selection, String[] args) {
		throw new UnsupportedOperationException("gunim serves files to read");
	}

	@Override
	public int update(Uri u, ContentValues values, String selection, String[] args) {
		throw new UnsupportedOperationException("gunim serves files to read");
	}
}
