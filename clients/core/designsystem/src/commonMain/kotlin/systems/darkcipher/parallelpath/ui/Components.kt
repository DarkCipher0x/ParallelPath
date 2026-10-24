package systems.darkcipher.parallelpath.ui

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.ErrorOutline
import androidx.compose.material.icons.outlined.Info
import androidx.compose.material.icons.outlined.Shield
import androidx.compose.material.icons.outlined.WarningAmber
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.OutlinedTextFieldDefaults
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.input.VisualTransformation
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp

/** A raised panel, the basic building block of every screen. */
@Composable
fun Panel(modifier: Modifier = Modifier, padding: PaddingValues = PaddingValues(20.dp), content: @Composable ColumnScope.() -> Unit) {
    Column(
        modifier
            .clip(RoundedCornerShape(Radius.large))
            .background(pp.panel)
            .border(1.dp, pp.line, RoundedCornerShape(Radius.large))
            .padding(padding),
        verticalArrangement = Arrangement.spacedBy(12.dp),
        content = content,
    )
}

@Composable
fun PrimaryButton(text: String, onClick: () -> Unit, modifier: Modifier = Modifier, enabled: Boolean = true, busy: Boolean = false) {
    Button(
        onClick = onClick, enabled = enabled && !busy, modifier = modifier,
        shape = RoundedCornerShape(Radius.small),
        colors = ButtonDefaults.buttonColors(containerColor = pp.accent, contentColor = pp.accentInk),
        contentPadding = PaddingValues(horizontal = 20.dp, vertical = 12.dp),
    ) {
        if (busy) {
            CircularProgressIndicator(Modifier.size(16.dp), color = pp.accentInk, strokeWidth = 2.dp)
            Spacer(Modifier.width(10.dp))
        }
        Text(text, fontWeight = FontWeight.SemiBold)
    }
}

@Composable
fun SecondaryButton(text: String, onClick: () -> Unit, modifier: Modifier = Modifier, enabled: Boolean = true, danger: Boolean = false) {
    OutlinedButton(
        onClick = onClick, enabled = enabled, modifier = modifier, shape = RoundedCornerShape(Radius.small),
        border = BorderStroke(1.dp, if (danger) pp.danger else pp.line2),
        colors = ButtonDefaults.outlinedButtonColors(contentColor = if (danger) pp.danger else pp.fg),
        contentPadding = PaddingValues(horizontal = 18.dp, vertical = 11.dp),
    ) { Text(text) }
}

@Composable
fun QuietButton(text: String, onClick: () -> Unit, modifier: Modifier = Modifier, color: Color = pp.accent) {
    TextButton(onClick = onClick, modifier = modifier) { Text(text, color = color) }
}

@Composable
fun Field(
    value: String, onChange: (String) -> Unit, label: String, modifier: Modifier = Modifier.fillMaxWidth(),
    secret: Boolean = false, singleLine: Boolean = true, supporting: String? = null, keyboard: KeyboardType = KeyboardType.Text,
    minLines: Int = 1, enabled: Boolean = true,
    // The keyboard's action key: "Done" runs this (e.g. the form's main button); without it a one-line field offers "Next".
    onDone: (() -> Unit)? = null,
) {
    val softKeyboard = androidx.compose.ui.platform.LocalSoftwareKeyboardController.current
    OutlinedTextField(
        value = value, onValueChange = onChange, label = { Text(label) }, modifier = modifier, enabled = enabled,
        singleLine = singleLine, minLines = minLines,
        visualTransformation = if (secret) PasswordVisualTransformation() else VisualTransformation.None,
        keyboardOptions = KeyboardOptions(keyboardType = if (secret) KeyboardType.Password else keyboard,
            imeAction = when { onDone != null -> ImeAction.Done; singleLine -> ImeAction.Next; else -> ImeAction.Default }),
        keyboardActions = if (onDone != null) KeyboardActions(onDone = { softKeyboard?.hide(); onDone() }) else KeyboardActions.Default,
        supportingText = supporting?.let { { Text(it, color = pp.fgMute) } },
        shape = RoundedCornerShape(Radius.small),
        colors = OutlinedTextFieldDefaults.colors(
            focusedBorderColor = pp.accent, unfocusedBorderColor = pp.line2, focusedLabelColor = pp.accent,
            cursorColor = pp.accent, focusedContainerColor = pp.bg2, unfocusedContainerColor = pp.bg2,
        ),
    )
}

enum class Tone { Info, Warn, Error, Ok }

/** An inline message: information, warning, error or success. */
@Composable
fun Banner(text: String, tone: Tone = Tone.Info, modifier: Modifier = Modifier.fillMaxWidth()) {
    val (color, icon) = when (tone) {
        Tone.Info -> pp.accent to Icons.Outlined.Info
        Tone.Warn -> pp.warm to Icons.Outlined.WarningAmber
        Tone.Error -> pp.danger to Icons.Outlined.ErrorOutline
        Tone.Ok -> pp.ok to Icons.Outlined.Shield
    }
    Row(
        modifier.clip(RoundedCornerShape(Radius.small)).background(color.copy(alpha = 0.12f))
            .border(1.dp, color.copy(alpha = 0.5f), RoundedCornerShape(Radius.small)).padding(12.dp),
        horizontalArrangement = Arrangement.spacedBy(10.dp), verticalAlignment = Alignment.Top,
    ) {
        Icon(icon, null, tint = color, modifier = Modifier.size(20.dp))
        Text(text, style = MaterialTheme.typography.bodyMedium, color = pp.fg)
    }
}

/** The small "post-quantum protected" pill shown wherever the protection matters. */
@Composable
fun PQBadge(modifier: Modifier = Modifier, text: String = "Post-quantum protected") {
    Row(
        modifier.clip(CircleShape).border(1.dp, pp.accent, CircleShape).padding(horizontal = 10.dp, vertical = 3.dp),
        verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        Icon(Icons.Outlined.Shield, null, tint = pp.accent, modifier = Modifier.size(14.dp))
        Text(text, color = pp.accent, style = MaterialTheme.typography.labelMedium)
    }
}

/** Round avatar with initials, coloured per person (stable). */
@Composable
fun Avatar(name: String, seed: String, size: Int = 38) {
    val palette = listOf(pp.accent, pp.warm, pp.violet, pp.ok, Color(0xFF5AA9F2), Color(0xFFF26D9A))
    val c = palette[(seed.hashCode() and 0x7fffffff) % palette.size]
    val initials = name.split(' ', '-').filter { it.firstOrNull()?.isLetterOrDigit() == true }.take(2)
        .joinToString("") { it.first().uppercase() }.ifBlank { "?" }
    Box(Modifier.size(size.dp).clip(CircleShape).background(c.copy(alpha = 0.22f)), contentAlignment = Alignment.Center) {
        Text(initials, color = c, fontWeight = FontWeight.Bold, fontSize = (size / 2.6).sp)
    }
}

/** A labelled section heading. */
@Composable
fun SectionTitle(text: String, modifier: Modifier = Modifier) {
    Text(text.uppercase(), modifier = modifier, color = pp.fgMute, style = MaterialTheme.typography.labelMedium, letterSpacing = 1.sp)
}

@Composable
fun EmptyState(icon: ImageVector, title: String, body: String, modifier: Modifier = Modifier) {
    Column(modifier.padding(32.dp), horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(10.dp)) {
        Icon(icon, null, tint = pp.fgMute, modifier = Modifier.size(44.dp))
        Text(title, style = MaterialTheme.typography.titleMedium, color = pp.fg)
        Text(body, style = MaterialTheme.typography.bodyMedium, color = pp.fgDim)
    }
}
