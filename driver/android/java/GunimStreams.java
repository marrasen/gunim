package gunim.android;

import android.content.ContentProviderClient;
import android.content.Context;
import android.content.res.AssetFileDescriptor;
import android.net.Uri;
import android.os.Parcel;
import android.os.ParcelFileDescriptor;
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
		private boolean closed;

		Open(ParcelFileDescriptor file, ContentProviderClient from) {
			this.file = file;
			this.from = from;
			this.word = wordOf(file);
		}

		synchronized void close() {
			if (closed) {
				return;
			}
			closed = true;
			for (ParcelFileDescriptor p : new ParcelFileDescriptor[] {word, file}) {
				try {
					if (p != null) {
						p.close();
					}
				} catch (IOException | RuntimeException e) {
					// It is let go of all the same.
				}
			}
			from.close();
		}

		/**
		 * end says how the sender ended, once its bytes have. checkError
		 * throws nothing while the sender's word has yet to come, as where
		 * Android ends a sender's process: that closes the bytes' pipe and
		 * the word's one after the other, in no set order. So end waits for
		 * the word, which a sender that closes its file gives first.
		 */
		int end() {
			for (int i = 0; i < WAITS; i++) {
				synchronized (this) {
					if (closed) {
						return FAILED;
					}
					if (word == null) {
						return UNKNOWN;
					}
					try {
						if (!hasCome(word)) {
							continue;
						}
						file.checkError();
						return DONE;
					} catch (ParcelFileDescriptor.FileDescriptorDetachedException e) {
						// The sender handed its end on, where Android
						// loses sight of it.
						return UNKNOWN;
					} catch (IOException | ErrnoException | RuntimeException e) {
						return FAILED;
					}
				}
			}
			return UNKNOWN;
		}
	}

	// WAITS is how often end waits for the sender's word, and WAIT how
	// long each time, in milliseconds.
	private static final int WAITS = 30, WAIT = 100;

	/** hasCome reports whether anything, or the end, has come to p, after a wait. */
	private static boolean hasCome(ParcelFileDescriptor p) throws ErrnoException {
		StructPollfd f = new StructPollfd();
		f.fd = p.getFileDescriptor();
		f.events = (short) OsConstants.POLLIN;
		return Os.poll(new StructPollfd[] {f}, WAIT) > 0;
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
			word = null;
		} finally {
			p.recycle();
			try {
				if (data != null) {
					data.close();
				}
			} catch (IOException | RuntimeException e) {
				// The copy is let go of all the same.
			}
		}
		return word;
	}

	private static final ConcurrentHashMap<Long, Open> open = new ConcurrentHashMap<>();
	private static final AtomicLong last = new AtomicLong();

	// How a sender ended, as Go numbers it.
	private static final int DONE = 0, UNKNOWN = 1, FAILED = 2;

	private GunimStreams() {
	}

	/**
	 * open opens u into number n of fds, ids and held: a descriptor for Go
	 * to read and close, the stream's id, and the file's size where it lies
	 * on a disk. It leaves -1 for one that fails to open.
	 *
	 * The provider is held in a way that Android calls unstable: it lives
	 * for as long as the file is open, and where it ends all the same, this
	 * program hears of a sender that gave up, where Android would end this
	 * program along with one held stably.
	 */
	static void open(Context app, Uri u, int n, int[] fds, long[] ids, long[] held) {
		fds[n] = -1;
		held[n] = -1;
		ContentProviderClient from = null;
		ParcelFileDescriptor file = null;
		try {
			from = app.getContentResolver().acquireUnstableContentProviderClient(u);
			if (from == null) {
				return;
			}
			AssetFileDescriptor part = from.openTypedAssetFileDescriptor(u, "*/*", null);
			if (part == null) {
				return;
			}
			file = part.getParcelFileDescriptor();
			if (part.getDeclaredLength() >= 0) {
				// It is a part of another file.
				return;
			}
			held[n] = file.getStatSize();
			int fd = ParcelFileDescriptor.dup(file.getFileDescriptor()).detachFd();
			long id = last.incrementAndGet();
			open.put(id, new Open(file, from));
			fds[n] = fd;
			ids[n] = id;
			file = null;
			from = null;
		} catch (Exception | StackOverflowError e) {
			// A provider may throw anything back through the binder, and
			// one file's failure is told as that file's.
			held[n] = -1;
		} finally {
			if (file != null) {
				new Open(file, from).close();
			} else if (from != null) {
				from.close();
			}
		}
	}

	/** end says how the sender of stream id ended, once its bytes have. */
	static int end(long id) {
		Open o = open.get(id);
		return o == null ? FAILED : o.end();
	}

	/** close lets stream id go. */
	static void close(long id) {
		Open o = open.remove(id);
		if (o != null) {
			o.close();
		}
	}
}
