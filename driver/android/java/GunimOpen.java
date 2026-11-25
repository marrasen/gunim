package gunim.android;

import android.content.ClipData;
import android.content.Context;
import android.content.Intent;
import android.content.pm.ActivityInfo;
import android.content.pm.PackageManager;
import android.database.Cursor;
import android.net.Uri;
import android.os.Bundle;
import android.os.ParcelFileDescriptor;
import android.provider.OpenableColumns;
import android.webkit.MimeTypeMap;
import java.util.ArrayList;
import java.util.List;
import java.util.Locale;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

/**
 * GunimOpen hands Go the files another application gives this one, and
 * those the document picker gives. Each arrives as a content address.
 * It opens them and reads their names apart from the UI thread, as a
 * file in the cloud can take a while, and Go copies them from there.
 */
final class GunimOpen {
	// Each batch has a thread of its own, so a provider that never answers
	// holds up no later file.
	private static final ExecutorService worker = Executors.newCachedThreadPool();

	private GunimOpen() {
	}

	/**
	 * opened hands over the files in an intent that started the activity
	 * or came to it. Android starts a task from Recents with the intent
	 * that first started it, whose files the program had already.
	 */
	static void opened(GunimActivity a, Intent i) {
		if (i == null || (i.getFlags() & Intent.FLAG_ACTIVITY_LAUNCHED_FROM_HISTORY) != 0) {
			return;
		}
		// Any application can send this activity an intent, so a program
		// that opens no files reads nothing of it, and a malformed one is
		// dropped: its extras can hold anything, or classes this program
		// lacks.
		Bundle meta = metaOf(a);
		String opens = opensFor(i.getAction(), meta.getString("gunim.opens"));
		if (opens.isEmpty()) {
			return;
		}
		try {
			List<Uri> uris = sentIn(i);
			if (!uris.isEmpty()) {
				hand(a, 0, uris, opens, meta.getBoolean("gunim.streams"));
			}
		} catch (RuntimeException e) {
			// What it holds is the other application's, and goes into no log.
			android.util.Log.w("gunim", "an intent sent to this program could not be read");
		}
	}

	/**
	 * opensFor returns what the program takes by action, of what -opens
	 * named: ANY for every file shared to it, where it named that, and
	 * otherwise the extensions, or "" for none.
	 */
	private static String opensFor(String action, String opens) {
		if (opens == null) {
			return "";
		}
		boolean shared = Intent.ACTION_SEND.equals(action) || Intent.ACTION_SEND_MULTIPLE.equals(action);
		StringBuilder exts = new StringBuilder();
		for (String e : opens.split(",")) {
			if (ANY.equals(e)) {
				if (shared) {
					return ANY;
				}
			} else if (!e.isEmpty()) {
				exts.append(exts.length() > 0 ? "," : "").append(e);
			}
		}
		return exts.toString();
	}

	private static final String ANY = "*";

	/** chosen hands over the files the document picker gave for code, as streams or as copies. */
	static void chosen(GunimActivity a, int code, Intent data, boolean streams) {
		hand(a, code, data == null ? new ArrayList<>() : chosenIn(data), "", streams);
	}

	/** sentIn returns the addresses another application sent in i. */
	@SuppressWarnings("deprecation")
	private static List<Uri> sentIn(Intent i) {
		List<Uri> uris = new ArrayList<>();
		String action = i.getAction();
		if (Intent.ACTION_VIEW.equals(action) && i.getData() != null) {
			uris.add(i.getData());
		} else if (Intent.ACTION_SEND.equals(action)) {
			Uri u = i.getParcelableExtra(Intent.EXTRA_STREAM);
			if (u != null) {
				uris.add(u);
			}
		} else if (Intent.ACTION_SEND_MULTIPLE.equals(action)) {
			ArrayList<Uri> many = i.getParcelableArrayListExtra(Intent.EXTRA_STREAM);
			if (many != null) {
				for (Uri u : many) {
					if (u != null) {
						uris.add(u);
					}
				}
			}
		}
		return uris;
	}

	/** chosenIn returns the addresses the picker gave: a clip for several. */
	private static List<Uri> chosenIn(Intent data) {
		List<Uri> uris = new ArrayList<>();
		ClipData clip = data.getClipData();
		if (clip != null) {
			for (int n = 0; n < clip.getItemCount(); n++) {
				Uri u = clip.getItemAt(n).getUri();
				if (u != null) {
					uris.add(u);
				}
			}
		} else if (data.getData() != null) {
			uris.add(data.getData());
		}
		return uris;
	}

	/** metaOf returns what gunimapk's -opens and -streams left the activity. */
	private static Bundle metaOf(GunimActivity a) {
		try {
			ActivityInfo info = a.getPackageManager().getActivityInfo(a.getComponentName(), PackageManager.GET_META_DATA);
			return info.metaData == null ? Bundle.EMPTY : info.metaData;
		} catch (PackageManager.NameNotFoundException e) {
			return Bundle.EMPTY;
		}
	}

	/**
	 * hand opens uris and gives Go each one's file descriptor and name.
	 * An address that fails to open goes as -1, for Go to report. Only a
	 * content address from another application opens: a file address, or
	 * one of a provider of this program's, would let a caller read the
	 * program's private files through it.
	 */
	private static void hand(Context c, int code, List<Uri> uris, String opens, boolean streams) {
		Context app = c.getApplicationContext();
		worker.execute(() -> {
			if (streams) {
				stream(app, code, uris, opens);
				return;
			}
			int[] fds = new int[uris.size()];
			StringBuilder names = new StringBuilder();
			for (int n = 0; n < uris.size(); n++) {
				Uri u = uris.get(n);
				if (n > 0) {
					names.append('\0');
				}
				names.append(nameOf(app, u).replace('\0', '_'));
				fds[n] = open(app, u);
			}
			Native.files(code, fds, names.toString(), opens);
		});
	}

	/** stream opens uris for Go to read as they come, with what is stated of each. */
	private static void stream(Context app, int code, List<Uri> uris, String opens) {
		int n = uris.size();
		int[] fds = new int[n];
		long[] ids = new long[n], stated = new long[n], held = new long[n];
		StringBuilder names = new StringBuilder(), types = new StringBuilder();
		for (int i = 0; i < n; i++) {
			Uri u = uris.get(i);
			fds[i] = -1;
			stated[i] = held[i] = -1;
			String type = null;
			if (isOthers(app, u)) {
				GunimStreams.open(app, u, i, fds, ids, held);
				stated[i] = sizeOf(app, u);
				type = typeOf(app, u);
			}
			names.append(i > 0 ? "\0" : "").append(nameOf(app, u).replace('\0', '_'));
			types.append(i > 0 ? "\0" : "").append(type == null ? "" : type.replace('\0', '_'));
		}
		Native.streams(code, fds, ids, stated, held, names.toString(), types.toString(), opens);
	}

	/** isOthers reports whether u is a content address of another application's. */
	private static boolean isOthers(Context app, Uri u) {
		String authority = withoutUser(u.getAuthority());
		if (!"content".equals(u.getScheme()) || authority == null) {
			return false;
		}
		android.content.pm.ProviderInfo p = app.getPackageManager().resolveContentProvider(authority,
			PackageManager.MATCH_DISABLED_COMPONENTS);
		// A provider Android hides from this program is another's.
		return !GunimFiles.authority(app).equals(authority) && (p == null || !app.getPackageName().equals(p.packageName));
	}

	private static int open(Context app, Uri u) {
		if (!isOthers(app, u)) {
			return -1;
		}
		try {
			ParcelFileDescriptor p = app.getContentResolver().openFileDescriptor(u, "r");
			return p == null ? -1 : p.detachFd();
		} catch (Exception e) {
			// A provider may throw anything back through the binder, and
			// one file's failure is told as that file's.
			android.util.Log.w("gunim", "a file sent to this program did not open");
			return -1;
		}
	}

	/** withoutUser returns an authority without the user, as 0@, Android also reads. */
	private static String withoutUser(String authority) {
		return authority == null ? null : authority.substring(authority.lastIndexOf('@') + 1);
	}

	/** nameOf asks the provider for a file's name, or takes the address's last part. */
	private static String nameOf(Context app, Uri u) {
		if (isOthers(app, u)) {
			try (Cursor c = app.getContentResolver().query(u, new String[] {OpenableColumns.DISPLAY_NAME}, null, null, null)) {
				if (c != null && c.moveToFirst() && !c.isNull(0)) {
					return c.getString(0);
				}
			} catch (Exception e) {
				android.util.Log.w("gunim", "a file sent to this program did not tell its name");
			}
		}
		String last = u.getLastPathSegment();
		return last == null ? "" : last;
	}

	/** sizeOf asks the provider for a file's size, or returns -1. */
	private static long sizeOf(Context app, Uri u) {
		try (Cursor c = app.getContentResolver().query(u, new String[] {OpenableColumns.SIZE}, null, null, null)) {
			if (c != null && c.moveToFirst() && !c.isNull(0)) {
				return c.getLong(0);
			}
		} catch (Exception e) {
			// It states none.
		}
		return -1;
	}

	/** typeOf asks the provider for a file's MIME type, or returns null. */
	private static String typeOf(Context app, Uri u) {
		try {
			return app.getContentResolver().getType(u);
		} catch (Exception e) {
			return null;
		}
	}

	/**
	 * typesOf returns the MIME types of patterns as *.ext, each ended by a
	 * NUL, or null to show every file: for no patterns, for a pattern of
	 * another form, and for an extension Android knows no type for.
	 */
	static String[] typesOf(String patterns) {
		List<String> types = new ArrayList<>();
		for (String p : patterns.split("\0")) {
			if (p.isEmpty()) {
				continue;
			}
			String type = p.startsWith("*.")
				? MimeTypeMap.getSingleton().getMimeTypeFromExtension(p.substring(2).toLowerCase(Locale.ROOT))
				: null;
			if (type == null) {
				return null;
			}
			if (!types.contains(type)) {
				types.add(type);
			}
		}
		return types.isEmpty() ? null : types.toArray(new String[0]);
	}
}
