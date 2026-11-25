package gunim.android;

import android.content.ClipData;
import android.content.Context;
import android.content.Intent;
import android.content.pm.ActivityInfo;
import android.content.pm.PackageManager;
import android.net.Uri;
import android.os.Bundle;
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
		} catch (RuntimeException | OutOfMemoryError | StackOverflowError e) {
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
			List<GunimStreams.Found> found = new ArrayList<>();
			try {
				for (Uri u : uris) {
					found.add(GunimStreams.open(app, u));
				}
				hand(code, found, opens, streams);
			} catch (RuntimeException | OutOfMemoryError | StackOverflowError e) {
				// There were too many, or too large a word of one.
				android.util.Log.w("gunim", "the files sent to this program could not be taken");
				for (GunimStreams.Found f : found) {
					f.close();
				}
				if (code != 0) {
					Native.noFiles(code, streams);
				}
			}
		});
	}

	/** hand gives Go the files found, which are Go's to close once it returns. */
	private static void hand(int code, List<GunimStreams.Found> found, String opens, boolean streams) {
		int n = found.size();
		int[] fds = new int[n];
		long[] ids = new long[n], stated = new long[n], held = new long[n];
		StringBuilder names = new StringBuilder(), types = new StringBuilder();
		for (int i = 0; i < n; i++) {
			GunimStreams.Found f = found.get(i);
			fds[i] = f.fd;
			ids[i] = f.id;
			stated[i] = f.stated;
			held[i] = f.held;
			names.append(i > 0 ? "\0" : "").append(f.name);
			types.append(i > 0 ? "\0" : "").append(f.type);
		}
		if (streams) {
			Native.streams(code, fds, ids, stated, held, names.toString(), types.toString(), opens);
		} else {
			Native.files(code, fds, ids, names.toString(), opens);
		}
	}

	/** isOthers reports whether u is a content address of another application's. */
	static boolean isOthers(Context app, Uri u) {
		String authority = withoutUser(u.getAuthority());
		if (!"content".equals(u.getScheme()) || authority == null) {
			return false;
		}
		android.content.pm.ProviderInfo p = app.getPackageManager().resolveContentProvider(authority,
			PackageManager.MATCH_DISABLED_COMPONENTS);
		// A provider Android hides from this program is another's.
		return !GunimFiles.authority(app).equals(authority) && (p == null || !app.getPackageName().equals(p.packageName));
	}

	/** withoutUser returns an authority without the user, as 0@, Android also reads. */
	private static String withoutUser(String authority) {
		return authority == null ? null : authority.substring(authority.lastIndexOf('@') + 1);
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
