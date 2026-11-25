package gunim.android;

import android.content.ContentProviderClient;
import android.content.Context;
import android.content.res.AssetFileDescriptor;
import android.database.Cursor;
import android.net.Uri;
import android.os.Parcel;
import android.os.ParcelFileDescriptor;
import android.provider.OpenableColumns;
import android.system.ErrnoException;
import android.system.Os;
import android.system.OsConstants;
import android.system.StructPollfd;
import java.io.IOException;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.atomic.AtomicLong;

/**
 * GunimStreams holds the files that Go reads as they come. Go reads a
 * descriptor of its own. Java keeps the one Android gave, which alone
 * hears how a sender ended: a pipe's end looks the same to a read whether
 * the sender finished or gave up.
 */
final class GunimStreams {
	/** Open is a file open, and the provider it comes from. */
	private static final class Open {
		private final ParcelFileDescriptor file;
		private final ContentProviderClient from;
		// word is where the sender's word of its end comes, or null where
		// none will.
		private final ParcelFileDescriptor word;
		private volatile boolean closed;
		// polling says end looks for the word.
		private boolean polling;

		Open(ParcelFileDescriptor file, ContentProviderClient from) {
			this.file = file;
			this.from = from;
			this.word = wordOf(file);
		}

		void close() {
			boolean inUse;
			synchronized (this) {
				if (closed) {
					return;
				}
				closed = true;
				inUse = polling;
			}
			if (!inUse) {
				letGo(word);
			}
			letGo(file);
			from.close();
		}

		/**
		 * end says how the sender ended, once its bytes have. checkError
		 * throws nothing while the sender's word has yet to come, as where
		 * Android ends a sender's process: that closes the bytes' pipe and
		 * the word's one after the other, in no set order. So end looks
		 * for the word itself, which a sender that closes its file gives
		 * first, and with wait waits for it a while.
		 */
		int end(boolean wait) {
			synchronized (this) {
				if (closed) {
					return FAILED;
				}
				if (word == null) {
					return UNKNOWN;
				}
				polling = true;
			}
			// close waits for no word, so this holds no lock, and lets go
			// of the word itself where it is closed meanwhile.
			boolean come = false, failed = false;
			try {
				for (int i = 0; i < (wait ? WAITS : 1) && !come && !closed; i++) {
					come = hasCome(word, wait ? WAIT : 0);
				}
			} catch (ErrnoException | RuntimeException e) {
				failed = true;
			}
			synchronized (this) {
				polling = false;
				if (closed) {
					letGo(word);
					return FAILED;
				}
				if (failed) {
					return FAILED;
				}
				if (!come) {
					return UNKNOWN;
				}
				try {
					file.checkError();
					return DONE;
				} catch (ParcelFileDescriptor.FileDescriptorDetachedException e) {
					// The sender handed its end on, where Android loses
					// sight of it.
					return UNKNOWN;
				} catch (IOException | RuntimeException e) {
					return FAILED;
				}
			}
		}
	}

	private static void letGo(ParcelFileDescriptor p) {
		try {
			if (p != null) {
				p.close();
			}
		} catch (IOException | RuntimeException e) {
			// It is let go of all the same.
		}
	}

	// WAITS is how often end waits for the sender's word, and WAIT how
	// long each time, in milliseconds.
	private static final int WAITS = 30, WAIT = 100;

	/** hasCome reports whether anything, or the end, has come to p, after a wait in milliseconds. */
	private static boolean hasCome(ParcelFileDescriptor p, int wait) throws ErrnoException {
		StructPollfd f = new StructPollfd();
		f.fd = p.getFileDescriptor();
		f.events = (short) OsConstants.POLLIN;
		return Os.poll(new StructPollfd[] {f}, wait) > 0;
	}

	/**
	 * wordOf returns a descriptor for the sender's word of its end, apart
	 * from file's own, or null. ParcelFileDescriptor keeps it to itself,
	 * and writes both to a parcel: 1, the file and the word, or 0 and the
	 * file alone. Where it wrote anything else, there is no word, and the
	 * file's end is not known.
	 */
	private static ParcelFileDescriptor wordOf(ParcelFileDescriptor file) {
		if (!file.canDetectErrors()) {
			return null;
		}
		Parcel p = Parcel.obtain();
		ParcelFileDescriptor data = null, word = null;
		try {
			file.writeToParcel(p, 0);
			int end = p.dataPosition();
			p.setDataPosition(0);
			if (p.readInt() == 1) {
				data = p.readFileDescriptor();
				word = p.readFileDescriptor();
				if (p.dataPosition() != end) {
					word.close();
					word = null;
				}
			}
		} catch (IOException | RuntimeException e) {
			letGo(word);
			word = null;
		} finally {
			p.recycle();
			letGo(data);
		}
		return word;
	}

	private static final ConcurrentHashMap<Long, Open> open = new ConcurrentHashMap<>();
	private static final AtomicLong last = new AtomicLong();

	// How a sender ended, as Go numbers it.
	private static final int DONE = 0, UNKNOWN = 1, FAILED = 2;

	private GunimStreams() {
	}

	/** Found is a file another application hands over, and what it says of it. */
	static final class Found {
		// fd is for Go to read and close, -1 for a file that failed to
		// open, and id the one the file is kept by.
		int fd = -1;
		long id;
		// stated is the size the application states, and held the size of
		// the open file, where it has one; each is -1 otherwise.
		long stated = -1, held = -1;
		String name = "", type = "";

		/** close lets a file go that Go has not taken. */
		void close() {
			if (fd >= 0) {
				letGo(ParcelFileDescriptor.adoptFd(fd));
				fd = -1;
			}
			GunimStreams.close(id);
		}
	}

	// The most of a name and of a type that is taken.
	private static final int LONGEST_NAME = 1000, LONGEST_TYPE = 255;

	/**
	 * open asks of u, where it is another application's, and opens it.
	 *
	 * The provider is held in a way that Android calls unstable: it lives
	 * for as long as the file is open, and where it ends all the same, this
	 * program hears of a sender that gave up, where Android would end this
	 * program along with one held stably, as ContentResolver's own calls
	 * hold it.
	 */
	static Found open(Context app, Uri u) {
		Found f = new Found();
		ContentProviderClient from = null;
		ParcelFileDescriptor file = null;
		try {
			f.name = cut(u.getLastPathSegment(), LONGEST_NAME);
			if (!GunimOpen.isOthers(app, u)) {
				return f;
			}
			from = app.getContentResolver().acquireUnstableContentProviderClient(u);
			if (from == null) {
				return f;
			}
			ask(from, u, f);
			AssetFileDescriptor part = from.openTypedAssetFileDescriptor(u, "*/*", null);
			if (part == null) {
				return f;
			}
			file = part.getParcelFileDescriptor();
			long size = file.getStatSize(), declared = part.getDeclaredLength();
			if (part.getStartOffset() != 0 || (declared >= 0 && size >= 0 && declared != size)) {
				// It is a part of another file.
				return f;
			}
			long id = last.incrementAndGet();
			open.put(id, new Open(file, from));
			f.id = id;
			file = null;
			from = null;
			f.held = declared >= 0 ? declared : size;
			f.fd = ParcelFileDescriptor.dup(open.get(id).file.getFileDescriptor()).detachFd();
		} catch (Exception | OutOfMemoryError | StackOverflowError e) {
			// A provider may send anything back through the binder, of any
			// size, and one file's failure is told as that file's.
			f.close();
		} finally {
			letGo(file);
			if (from != null) {
				from.close();
			}
		}
		if (f.fd < 0) {
			android.util.Log.w("gunim", "a file sent to this program did not open");
		}
		return f;
	}

	/** ask asks the provider for u's name, size and type. It may tell none. */
	private static void ask(ContentProviderClient from, Uri u, Found f) {
		try (Cursor c = from.query(u, new String[] {OpenableColumns.DISPLAY_NAME, OpenableColumns.SIZE}, null, null, null)) {
			if (c != null && c.moveToFirst()) {
				int name = c.getColumnIndex(OpenableColumns.DISPLAY_NAME), size = c.getColumnIndex(OpenableColumns.SIZE);
				if (size >= 0 && !c.isNull(size)) {
					f.stated = c.getLong(size);
				}
				if (name >= 0 && !c.isNull(name)) {
					f.name = cut(c.getString(name), LONGEST_NAME);
				}
			}
		} catch (Exception | OutOfMemoryError | StackOverflowError e) {
			// What it says of itself is the other application's, and goes
			// into no log.
			android.util.Log.w("gunim", "a file sent to this program did not tell its name");
		}
		try {
			String type = from.getType(u);
			// One too long is none, as a part of it is another.
			f.type = type == null || type.length() > LONGEST_TYPE ? "" : type.replace('\0', '_');
		} catch (Exception | OutOfMemoryError | StackOverflowError e) {
			// It states none.
		}
	}

	/** cut returns at most n of s, with no NUL, or "" for null. A name keeps its end, the extension. */
	private static String cut(String s, int n) {
		if (s == null) {
			return "";
		}
		return (s.length() > n ? s.substring(s.length() - n) : s).replace('\0', '_');
	}

	/** end says how the sender of stream id ended, once its bytes have. */
	static int end(long id, boolean wait) {
		Open o = open.get(id);
		return o == null ? FAILED : o.end(wait);
	}

	/** close lets stream id go. */
	static void close(long id) {
		Open o = open.remove(id);
		if (o != null) {
			o.close();
		}
	}
}
