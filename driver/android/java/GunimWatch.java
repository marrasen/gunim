package gunim.android;

import android.app.Activity;
import android.app.Application;
import android.content.ContentProvider;
import android.content.ContentValues;
import android.content.Context;
import android.database.Cursor;
import android.net.Uri;
import android.os.Bundle;
import java.util.ArrayList;
import java.util.HashSet;
import java.util.List;

/**
 * GunimWatch knows every activity of the program's in this process,
 * gunim's and any of the program's own. It is a provider, and provides
 * nothing: Android makes one before the first activity, so it misses
 * none.
 */
public final class GunimWatch extends ContentProvider implements Application.ActivityLifecycleCallbacks {
	// live are the activities there are, finished ones too until they are
	// destroyed, and shown those that can be seen. The UI thread keeps both.
	private static final List<Activity> live = new ArrayList<>();
	private static final HashSet<Activity> shown = new HashSet<>();

	/** live returns the activities there are, oldest first. */
	static List<Activity> live() {
		return new ArrayList<>(live);
	}

	/** isShown reports whether a can be seen. */
	static boolean isShown(Activity a) {
		return shown.contains(a);
	}

	/** isAlone reports whether there is no activity but c, where c is one. */
	static boolean isAlone(Context c) {
		for (Activity a : live) {
			if (a != c) {
				return false;
			}
		}
		return true;
	}

	@Override
	public boolean onCreate() {
		Context app = getContext().getApplicationContext();
		if (app instanceof Application) {
			((Application) app).registerActivityLifecycleCallbacks(this);
		}
		return true;
	}

	@Override
	public void onActivityCreated(Activity a, Bundle state) {
		live.add(a);
	}

	@Override
	public void onActivityStarted(Activity a) {
		shown.add(a);
	}

	@Override
	public void onActivityResumed(Activity a) {
	}

	@Override
	public void onActivityPaused(Activity a) {
	}

	@Override
	public void onActivityStopped(Activity a) {
		shown.remove(a);
	}

	@Override
	public void onActivitySaveInstanceState(Activity a, Bundle state) {
	}

	@Override
	public void onActivityDestroyed(Activity a) {
		shown.remove(a);
		live.remove(a);
	}

	// It provides nothing.

	@Override
	public String getType(Uri uri) {
		return null;
	}

	@Override
	public Cursor query(Uri uri, String[] projection, String selection, String[] args, String order) {
		return null;
	}

	@Override
	public Uri insert(Uri uri, ContentValues values) {
		return null;
	}

	@Override
	public int delete(Uri uri, String selection, String[] args) {
		return 0;
	}

	@Override
	public int update(Uri uri, ContentValues values, String selection, String[] args) {
		return 0;
	}
}
