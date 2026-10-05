package gunim.android;

import android.content.ContentProvider;
import android.content.ContentValues;
import android.content.Context;
import android.database.Cursor;
import android.database.MatrixCursor;
import android.net.Uri;
import android.os.ParcelFileDescriptor;
import android.provider.OpenableColumns;
import android.webkit.MimeTypeMap;
import java.io.File;
import java.io.FileNotFoundException;

/**
 * GunimFiles hands the files a program shares to the application the
 * user shares them with. A file's address holds its path, as
 * content://org.gunim.music.gunim.files/sdcard/Download/song.mp3, and
 * the provider is private: an application reads only the files a share
 * granted it, and only to read.
 */
public final class GunimFiles extends ContentProvider {
	/** authority returns the provider's authority, from the package's name. */
	static String authority(Context c) {
		return c.getPackageName() + ".gunim.files";
	}

	/** uriOf returns the address of the file at path. */
	static Uri uriOf(Context c, String path) {
		return new Uri.Builder().scheme("content").authority(authority(c)).path(path).build();
	}

	/** typeOf returns the MIME type of the file at path, from its extension. */
	static String typeOf(String path) {
		String name = new File(path).getName();
		int dot = name.lastIndexOf('.');
		String type = null;
		if (dot >= 0) {
			type = MimeTypeMap.getSingleton().getMimeTypeFromExtension(name.substring(dot + 1).toLowerCase(java.util.Locale.ROOT));
		}
		return type != null ? type : "application/octet-stream";
	}

	/**
	 * commonType returns a type that covers both a and b: the type
	 * itself where they match, its kind where only that matches, as
	 * audio/*, and anything otherwise. A null a is the first file's.
	 */
	static String commonType(String a, String b) {
		if (a == null || a.equals(b)) {
			return b;
		}
		String kind = a.substring(0, a.indexOf('/') + 1);
		return b.startsWith(kind) ? kind + "*" : "*/*";
	}

	private static File fileOf(Uri uri) throws FileNotFoundException {
		String path = uri.getPath();
		if (path == null) {
			throw new FileNotFoundException(uri.toString());
		}
		File f = new File(path);
		if (!f.isFile()) {
			throw new FileNotFoundException(path);
		}
		return f;
	}

	@Override
	public boolean onCreate() {
		return true;
	}

	@Override
	public String getType(Uri uri) {
		return typeOf(uri.getPath() != null ? uri.getPath() : "");
	}

	@Override
	public ParcelFileDescriptor openFile(Uri uri, String mode) throws FileNotFoundException {
		if (!"r".equals(mode)) {
			throw new SecurityException("gunim shares files to read: mode " + mode);
		}
		return ParcelFileDescriptor.open(fileOf(uri), ParcelFileDescriptor.MODE_READ_ONLY);
	}

	/** query answers the name and size a receiving application asks about. */
	@Override
	public Cursor query(Uri uri, String[] projection, String selection, String[] args, String order) {
		File f;
		try {
			f = fileOf(uri);
		} catch (FileNotFoundException e) {
			return null;
		}
		if (projection == null) {
			projection = new String[] {OpenableColumns.DISPLAY_NAME, OpenableColumns.SIZE};
		}
		java.util.ArrayList<String> cols = new java.util.ArrayList<>();
		java.util.ArrayList<Object> row = new java.util.ArrayList<>();
		for (String col : projection) {
			if (OpenableColumns.DISPLAY_NAME.equals(col)) {
				cols.add(col);
				row.add(f.getName());
			} else if (OpenableColumns.SIZE.equals(col)) {
				cols.add(col);
				row.add(f.length());
			}
		}
		MatrixCursor c = new MatrixCursor(cols.toArray(new String[0]), 1);
		c.addRow(row.toArray());
		return c;
	}

	// The files are the program's own, to read: the rest of a provider's
	// calls change nothing.

	@Override
	public Uri insert(Uri uri, ContentValues values) {
		throw new UnsupportedOperationException("gunim shares files to read");
	}

	@Override
	public int delete(Uri uri, String selection, String[] args) {
		throw new UnsupportedOperationException("gunim shares files to read");
	}

	@Override
	public int update(Uri uri, ContentValues values, String selection, String[] args) {
		throw new UnsupportedOperationException("gunim shares files to read");
	}
}
