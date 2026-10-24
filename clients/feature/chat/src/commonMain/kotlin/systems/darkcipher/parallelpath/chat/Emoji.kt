package systems.darkcipher.parallelpath.chat

/** A built-in emoji set (no emoji service or download needed), grouped like a phone keyboard. */
internal object Emoji {
    /** One-tap reactions shown above a message. */
    val quick = listOf("👍", "❤️", "😊", "😂", "🙏", "👌", "😮", "😢")

    val categories: List<Pair<String, List<String>>> = listOf(
        "Smileys" to "😀 😃 😄 😁 😆 😅 🤣 😂 🙂 😉 😊 😇 🥰 😍 🤩 😘 😗 😚 😋 😛 😜 🤪 😝 🤗 🤭 🤫 🤔 🤐 🤨 😐 😑 😶 😏 😒 🙄 😬 😌 😔 😪 😴 😷 🤒 🤕 🥵 🥶 🥴 😵 🤯 🥳 😎 🤓 🧐 😕 😟 🙁 😮 😯 😲 😳 🥺 😦 😧 😨 😰 😥 😢 😭 😱 😖 😣 😞 😓 😩 😫 🥱 😤 😡 😠".split(' '),
        "People" to "👋 🤚 ✋ 🖖 👌 🤌 ✌️ 🤞 🤟 🤘 🤙 👈 👉 👆 👇 👍 👎 ✊ 👊 👏 🙌 👐 🤲 🤝 🙏 💪 👶 🧒 👦 👧 🧑 👨 👩 🧓 👴 👵 👪 👨‍👩‍👧 👨‍👩‍👦 👩‍👧 👨‍👧 👩‍👦 👨‍👦 🤰 🧑‍🍼 🫶".split(' '),
        "Hearts" to "❤️ 🧡 💛 💚 💙 💜 🤎 🖤 🤍 💔 ❣️ 💕 💞 💓 💗 💖 💘 💝 ✨ ⭐ 🌟 💫 🔥 🌈".split(' '),
        "Activities" to "⚽ 🏀 🏈 ⚾ 🎾 🏐 🏉 🎱 🏓 🏸 🥅 ⛳ 🎣 🤿 🎽 🛹 🛼 ⛸️ 🎿 🏊 🚴 🧗 🎨 🎭 🎤 🎧 🎹 🥁 🎸 🎻 🎮 🎲 🧩 📚 ✏️ 🎒 🏫 🎂 🎁 🎈 🎉 🎄".split(' '),
        "Food" to "🍎 🍌 🍇 🍓 🍉 🍊 🍐 🍒 🥝 🥕 🌽 🥦 🍞 🧀 🥚 🍳 🥞 🍔 🍟 🍕 🌭 🥪 🌮 🍝 🍜 🍲 🍣 🍦 🍩 🍪 🍫 🍬 🥤 🧃 ☕ 🍼 🥛".split(' '),
        "Travel" to "🚗 🚕 🚙 🚌 🚎 🚑 🚒 🚓 🚲 🛴 🚆 🚇 ✈️ 🚀 🛳️ ⛵ 🏠 🏡 🏢 🏥 🏖️ 🏕️ 🌍 🗺️ 🌞 🌙 ⛅ 🌧️ ❄️ ⛄ 🌊".split(' '),
        "Objects" to "📱 💻 ⌚ 📷 📞 ⏰ 📅 🗓️ 📌 📎 ✂️ 🔑 🏷️ 💊 🩹 🧸 🎀 👕 👖 👗 👟 🧢 🧦 🛏️ 🧴 🪥 📝 📄 💷 💶 💵 🧾".split(' '),
        "Symbols" to "✅ ☑️ ✔️ ❌ ❎ ➕ ➖ ❓ ❗ ‼️ ⚠️ 🚫 ⛔ 🔴 🟠 🟡 🟢 🔵 🟣 ⚪ ⚫ 🆗 🆕 🆒 🔜 🔙 ⏳ ⌛ 💤 🔔 🔕 ♻️".split(' '),
    )
}
