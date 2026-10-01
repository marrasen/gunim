package gunim.android;

import android.os.Build;
import android.text.Editable;
import android.text.InputType;
import android.text.Selection;
import android.text.SpannableStringBuilder;
import android.view.KeyEvent;
import android.view.View;
import android.view.inputmethod.BaseInputConnection;
import android.view.inputmethod.EditorInfo;
import android.view.inputmethod.InputConnection;
import android.view.inputmethod.InputMethodManager;

/**
 * GunimInput is the soft keyboard's side of gunim's text. It keeps a
 * copy of the focused node's text, which Android's BaseInputConnection
 * edits as the keyboard asks, so every question the keyboard asks is
 * answered at once, here on the UI thread.
 *
 * Each change to the copy goes to Go as one edit: the UTF-8 bytes it
 * replaces, what it puts there, and the selection and composition after
 * it, all in bytes into Go's whole text. Edits are numbered. Go sends
 * each new state of the node's text back with the number of the last
 * edit it holds. A state behind the edits sent is left alone, since
 * the edits after it are on their way. A state level with them that
 * differs from the copy is a change of Go's, such as text the program
 * set, and the keyboard starts over from it.
 *
 * A node with no text of its own to show, such as a terminal, has no
 * state. The keyboard then sends keys, which reach Go as key presses.
 */
final class GunimInput {
	private final View view;
	private final InputMethodManager imm;
	private final SpannableStringBuilder editable = new SpannableStringBuilder();

	// The copy as Go last heard of it: its text, where the text starts
	// in Go's whole text in bytes, and the selection and the
	// composition in it, in UTF-16 units, -1 for no composition.
	private String text = "";
	private int start;
	private int selA, selB;
	private int compA = -1, compB = -1;
	private boolean hasState, multiline, secret;
	// seq numbers the edits sent; batch counts the keyboard's batch
	// edits open, whose changes go together as they close.
	private long seq;
	private int batch;

	GunimInput(View view, InputMethodManager imm) {
		this.view = view;
		this.imm = imm;
	}

	/** hasState reports whether a node with text of its own has focus. */
	boolean hasState() {
		return hasState;
	}

	/** connect describes the text to the keyboard and connects it. */
	InputConnection connect(EditorInfo out) {
		if (!hasState) {
			out.inputType = InputType.TYPE_NULL;
		} else {
			int t = InputType.TYPE_CLASS_TEXT;
			if (secret) {
				t |= InputType.TYPE_TEXT_VARIATION_PASSWORD;
			} else {
				t |= InputType.TYPE_TEXT_FLAG_AUTO_CORRECT | InputType.TYPE_TEXT_FLAG_CAP_SENTENCES;
			}
			if (multiline) {
				t |= InputType.TYPE_TEXT_FLAG_MULTI_LINE;
			}
			out.inputType = t;
		}
		out.imeOptions = EditorInfo.IME_FLAG_NO_FULLSCREEN | EditorInfo.IME_FLAG_NO_EXTRACT_UI
			| (multiline ? EditorInfo.IME_FLAG_NO_ENTER_ACTION : EditorInfo.IME_ACTION_DONE);
		out.initialSelStart = Math.min(selA, selB);
		out.initialSelEnd = Math.max(selA, selB);
		if (Build.VERSION.SDK_INT >= 30) {
			out.setInitialSurroundingText(editable);
		}
		batch = 0;
		return new Connection();
	}

	/** setState takes a state of the node's text from Go. */
	void setState(String s, int start, int a, int b, int ca, int cb, boolean multiline, boolean secret, long seq) {
		boolean kind = !hasState || multiline != this.multiline || secret != this.secret;
		hasState = true;
		this.multiline = multiline;
		this.secret = secret;
		if (seq != this.seq || batch > 0) {
			return; // behind the edits sent, or the keyboard is mid-edit
		}
		if (ca == cb) {
			ca = cb = -1;
		}
		if (kind || start != this.start || !s.equals(text)) {
			this.start = start;
			text = s;
			selA = a;
			selB = b;
			compA = compB = -1;
			editable.clearSpans();
			editable.replace(0, editable.length(), s);
			Selection.setSelection(editable, a, b);
			imm.restartInput(view);
			return;
		}
		if (a != selA || b != selB) {
			selA = a;
			selB = b;
			Selection.setSelection(editable, a, b);
			imm.updateSelection(view, Math.min(a, b), Math.max(a, b), compA, compB);
		}
	}

	/** clearState leaves the keyboard with no text of Go's. */
	void clearState() {
		hasState = false;
		text = "";
		selA = selB = 0;
		compA = compB = -1;
		editable.clearSpans();
		editable.clear();
		imm.restartInput(view);
	}

	/** changed sends the copy's change, once no batch edit is open. */
	private void changed() {
		if (batch == 0) {
			sync();
		}
	}

	/** sync sends Go the change since the copy was last sent, if any. */
	private void sync() {
		String now = editable.toString();
		int ss = Selection.getSelectionStart(editable);
		int se = Selection.getSelectionEnd(editable);
		if (ss < 0 || se < 0) {
			ss = se = now.length();
		}
		int cs = BaseInputConnection.getComposingSpanStart(editable);
		int ce = BaseInputConnection.getComposingSpanEnd(editable);
		if (cs > ce) {
			int t = cs;
			cs = ce;
			ce = t;
		}
		if (cs < 0 || cs == ce) {
			cs = ce = -1;
		}
		if (!hasState) {
			typed(now, cs);
			return;
		}
		if (now.equals(text) && ss == selA && se == selB && cs == compA && ce == compB) {
			return;
		}
		// The text that differs, kept whole at a surrogate pair.
		int p = 0;
		int most = Math.min(text.length(), now.length());
		while (p < most && text.charAt(p) == now.charAt(p)) {
			p++;
		}
		if (p > 0 && Character.isHighSurrogate(text.charAt(p - 1))) {
			p--;
		}
		int q = 0;
		while (q < text.length() - p && q < now.length() - p
			&& text.charAt(text.length() - 1 - q) == now.charAt(now.length() - 1 - q)) {
			q++;
		}
		if (q > 0 && Character.isLowSurrogate(text.charAt(text.length() - q))) {
			q--;
		}
		String with = now.substring(p, now.length() - q);
		int r0 = start + utf8(text, 0, p);
		int r1 = start + utf8(text, 0, text.length() - q);
		int anchor = start + utf8(now, 0, ss);
		int caret = start + utf8(now, 0, se);
		int c0 = cs < 0 ? caret : start + utf8(now, 0, cs);
		int c1 = cs < 0 ? caret : start + utf8(now, 0, ce);
		seq++;
		Native.edit(with, r0, r1, anchor, caret, c0, c1, seq);
		text = now;
		selA = ss;
		selB = se;
		compA = cs;
		compB = ce;
	}

	/**
	 * typed sends text a keyboard commits with no state, which it seldom
	 * does with keys asked for, as typed text, and empties the copy.
	 */
	private void typed(String now, int cs) {
		if (cs >= 0 || now.isEmpty()) {
			return;
		}
		Native.text(now);
		editable.clearSpans();
		editable.clear();
	}

	/** utf8 returns how many UTF-8 bytes s takes from from to to. */
	static int utf8(String s, int from, int to) {
		int n = 0;
		for (int i = from; i < to; i++) {
			char c = s.charAt(i);
			if (c < 0x80) {
				n += 1;
			} else if (c < 0x800) {
				n += 2;
			} else if (Character.isHighSurrogate(c) && i + 1 < to && Character.isLowSurrogate(s.charAt(i + 1))) {
				n += 4;
				i++;
			} else {
				n += 3;
			}
		}
		return n;
	}

	/** Connection edits the copy as the keyboard asks. */
	private final class Connection extends BaseInputConnection {
		Connection() {
			super(view, true);
		}

		@Override
		public Editable getEditable() {
			return editable;
		}

		@Override
		public boolean beginBatchEdit() {
			batch++;
			return true;
		}

		@Override
		public boolean endBatchEdit() {
			if (batch > 0 && --batch == 0) {
				sync();
			}
			return batch > 0;
		}

		@Override
		public boolean commitText(CharSequence s, int cursor) {
			boolean ok = super.commitText(s, cursor);
			changed();
			return ok;
		}

		@Override
		public boolean setComposingText(CharSequence s, int cursor) {
			boolean ok = super.setComposingText(s, cursor);
			changed();
			return ok;
		}

		@Override
		public boolean setComposingRegion(int a, int b) {
			boolean ok = super.setComposingRegion(a, b);
			changed();
			return ok;
		}

		@Override
		public boolean finishComposingText() {
			boolean ok = super.finishComposingText();
			changed();
			return ok;
		}

		@Override
		public boolean deleteSurroundingText(int before, int after) {
			boolean ok = super.deleteSurroundingText(before, after);
			changed();
			return ok;
		}

		@Override
		public boolean deleteSurroundingTextInCodePoints(int before, int after) {
			boolean ok = super.deleteSurroundingTextInCodePoints(before, after);
			changed();
			return ok;
		}

		@Override
		public boolean setSelection(int a, int b) {
			boolean ok = super.setSelection(a, b);
			changed();
			return ok;
		}

		@Override
		public boolean performEditorAction(int action) {
			Native.key(true, KeyEvent.KEYCODE_ENTER, 0, 0, 0);
			Native.key(false, KeyEvent.KEYCODE_ENTER, 0, 0, 0);
			return true;
		}
	}
}
