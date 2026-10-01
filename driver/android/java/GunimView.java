package gunim.android;

import android.content.Context;
import android.view.KeyEvent;
import android.view.MotionEvent;
import android.view.SurfaceHolder;
import android.view.SurfaceView;
import android.view.inputmethod.EditorInfo;
import android.view.inputmethod.InputConnection;
import android.view.inputmethod.InputMethodManager;

/**
 * GunimView is the surface gunim draws every window into. It hands Go
 * the surface, the first finger's touches, the keys, and through
 * GunimInput the soft keyboard.
 */
final class GunimView extends SurfaceView implements SurfaceHolder.Callback {
	final GunimInput input;
	private final InputMethodManager imm;
	// pointer is the id of the finger that drives the pointer, or -1.
	private int pointer = -1;
	// tapped is when a finger last touched, for showing the keyboard
	// only for a text field the user tapped.
	private long tapped;

	GunimView(Context c) {
		super(c);
		imm = (InputMethodManager) c.getSystemService(Context.INPUT_METHOD_SERVICE);
		input = new GunimInput(this, imm);
		getHolder().addCallback(this);
		setFocusable(true);
		setFocusableInTouchMode(true);
	}

	@Override
	public void surfaceCreated(SurfaceHolder h) {}

	@Override
	public void surfaceChanged(SurfaceHolder h, int format, int width, int height) {
		float density = getResources().getDisplayMetrics().density;
		float rate = getDisplay() != null ? getDisplay().getRefreshRate() : 60;
		Native.metrics(density, rate);
		Native.surfaceChanged(h.getSurface(), width, height);
	}

	@Override
	public void surfaceDestroyed(SurfaceHolder h) {
		Native.surfaceDestroyed();
	}

	@Override
	public boolean onTouchEvent(MotionEvent e) {
		int action = e.getActionMasked();
		switch (action) {
		case MotionEvent.ACTION_DOWN:
			pointer = e.getPointerId(0);
			tapped = android.os.SystemClock.uptimeMillis();
			requestFocus();
			Native.touch(0, e.getX(0), e.getY(0), e.getEventTime());
			return true;
		case MotionEvent.ACTION_MOVE: {
			int i = e.findPointerIndex(pointer);
			if (i >= 0) {
				Native.touch(1, e.getX(i), e.getY(i), e.getEventTime());
			}
			return true;
		}
		case MotionEvent.ACTION_POINTER_UP:
		case MotionEvent.ACTION_UP: {
			int i = e.getActionIndex();
			if (e.getPointerId(i) == pointer) {
				Native.touch(2, e.getX(i), e.getY(i), e.getEventTime());
				pointer = -1;
			}
			return true;
		}
		case MotionEvent.ACTION_CANCEL:
			if (pointer >= 0) {
				Native.touch(3, 0, 0, e.getEventTime());
				pointer = -1;
			}
			return true;
		}
		return true;
	}

	@Override
	public boolean onKeyDown(int code, KeyEvent e) {
		if (code != KeyEvent.KEYCODE_BACK && e.isSystem()) {
			return super.onKeyDown(code, e);
		}
		Native.key(true, code, e.getMetaState(), e.getUnicodeChar(), e.getRepeatCount());
		return true;
	}

	@Override
	public boolean onKeyUp(int code, KeyEvent e) {
		if (code != KeyEvent.KEYCODE_BACK && e.isSystem()) {
			return super.onKeyUp(code, e);
		}
		Native.key(false, code, e.getMetaState(), 0, 0);
		return true;
	}

	@Override
	public void onWindowFocusChanged(boolean focused) {
		super.onWindowFocusChanged(focused);
		Native.focus(focused);
	}

	@Override
	public boolean onCheckIsTextEditor() {
		return input.hasState();
	}

	@Override
	public InputConnection onCreateInputConnection(EditorInfo out) {
		return input.connect(out);
	}

	/**
	 * showKeyboard shows the soft keyboard for a text field the user has
	 * just tapped, as Android's own fields do, and hides it. A field
	 * focused some other way, as when a window opens, leaves the
	 * keyboard down until it is tapped, and a node with no text of its
	 * own, which takes keys from a hardware keyboard, never shows it.
	 */
	void showKeyboard(boolean show) {
		long since = android.os.SystemClock.uptimeMillis() - tapped;
		if (show && input.hasState() && since < tapWindow) {
			requestFocus();
			imm.showSoftInput(this, 0);
		} else if (!show) {
			imm.hideSoftInputFromWindow(getWindowToken(), 0);
		}
	}

	// tapWindow is how long after a tap a field taking the keyboard
	// counts as tapped, in milliseconds.
	private static final long tapWindow = 1500;
}
