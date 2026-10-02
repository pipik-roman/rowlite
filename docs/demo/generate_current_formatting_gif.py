import os
import sys
import subprocess
import shutil
import tempfile
from PIL import Image, ImageDraw, ImageFont

# Canvas parameters
WIDTH = 1000
HEIGHT = 600
FPS = 15
PADDING = 24
TOP_BAR_HEIGHT = 40

# Catppuccin Mocha Palette
BG_COLOR = (30, 30, 46)          # #1e1e2e Base
BAR_COLOR = (24, 24, 37)         # #181825 Mantle
TEXT_COLOR = (205, 214, 244)     # #cdd6f4 Text
SUBTEXT_COLOR = (166, 173, 200)  # #a6adc8 Subtext0
OVERLAY_COLOR = (108, 112, 134)  # #6c7086 Overlay0 (comments)
PROMPT_COLOR = (137, 180, 250)   # #89b4fa Blue
CMD_COLOR = (203, 166, 247)      # #cba6f7 Mauve
SUB_CMD_COLOR = (137, 220, 235)  # #89dceb Sky
ARG_COLOR = (166, 227, 161)      # #a6e3a1 Green
FLAG_COLOR = (250, 179, 135)     # #fab387 Peach
BORDER_COLOR = (88, 91, 112)     # #585b70 Surface2
HEADER_COLOR = (249, 226, 175)   # #f9e2af Yellow
WARN_COLOR = (243, 139, 168)     # #f38ba8 Red/Coral
CURSOR_COLOR = (245, 224, 220)   # #f5e0dc Rosewater

# Window control dots
RED_DOT = (243, 139, 168)
YELLOW_DOT = (249, 226, 175)
GREEN_DOT = (166, 227, 161)

def find_font():
    candidates = [
        r"C:\Windows\Fonts\DejaVuSansMono.ttf",
        r"C:\Windows\Fonts\LiberationMono-Regular.ttf",
        r"C:\Windows\Fonts\consola.ttf"
    ]
    for c in candidates:
        if os.path.exists(c):
            return c
    return None

def find_font_bold():
    candidates = [
        r"C:\Windows\Fonts\DejaVuSansMono-Bold.ttf",
        r"C:\Windows\Fonts\LiberationMono-Bold.ttf",
        r"C:\Windows\Fonts\consolab.ttf"
    ]
    for c in candidates:
        if os.path.exists(c):
            return c
    return None

FONT_PATH = find_font()
FONT_BOLD_PATH = find_font_bold()
FONT_SIZE = 14
FONT = ImageFont.truetype(FONT_PATH, FONT_SIZE) if FONT_PATH else ImageFont.load_default()
FONT_BOLD = ImageFont.truetype(FONT_BOLD_PATH, FONT_SIZE) if FONT_BOLD_PATH else FONT
TITLE_FONT = ImageFont.truetype(FONT_PATH, 13) if FONT_PATH else FONT

LINE_HEIGHT = 20
MAX_VISIBLE_LINES = (HEIGHT - TOP_BAR_HEIGHT - 20) // LINE_HEIGHT

def create_base_terminal(page_info=""):
    img = Image.new("RGBA", (WIDTH, HEIGHT), BG_COLOR)
    draw = ImageDraw.Draw(img)
    
    # Top bar
    draw.rectangle([0, 0, WIDTH, TOP_BAR_HEIGHT], fill=BAR_COLOR)
    
    # Window controls
    dot_radius = 6
    y_center = TOP_BAR_HEIGHT // 2
    draw.ellipse([20, y_center - dot_radius, 20 + dot_radius*2, y_center + dot_radius], fill=RED_DOT)
    draw.ellipse([38, y_center - dot_radius, 38 + dot_radius*2, y_center + dot_radius], fill=YELLOW_DOT)
    draw.ellipse([56, y_center - dot_radius, 56 + dot_radius*2, y_center + dot_radius], fill=GREEN_DOT)
    
    # Title
    title = "rowlite — Current Baseline: Unconstrained Overflow & Terminal Wrapping"
    if page_info:
        title += f" ({page_info})"
    bbox = draw.textbbox((0, 0), title, font=TITLE_FONT)
    tw = bbox[2] - bbox[0]
    draw.text(((WIDTH - tw) // 2, (TOP_BAR_HEIGHT - (bbox[3] - bbox[1])) // 2), title, font=TITLE_FONT, fill=SUBTEXT_COLOR)
    
    # Border
    draw.rectangle([0, 0, WIDTH - 1, HEIGHT - 1], outline=BORDER_COLOR, width=1)
    return img

def render_viewport(lines, start_line_idx=0, cursor_pos=None, page_info=""):
    img = create_base_terminal(page_info)
    draw = ImageDraw.Draw(img)
    
    start_y = TOP_BAR_HEIGHT + 12
    start_x = PADDING
    
    visible_slice = lines[start_line_idx : start_line_idx + MAX_VISIBLE_LINES]

    for row_idx, line in enumerate(visible_slice):
        curr_x = start_x
        curr_y = start_y + row_idx * LINE_HEIGHT
            
        for text, color, is_bold in line:
            f = FONT_BOLD if is_bold else FONT
            draw.text((curr_x, curr_y), text, font=f, fill=color)
            bbox = draw.textbbox((curr_x, curr_y), text, font=f)
            curr_x += (bbox[2] - bbox[0])
            
        if cursor_pos and cursor_pos[0] - start_line_idx == row_idx:
            c_w = 8
            draw.rectangle([cursor_pos[1], curr_y + 2, cursor_pos[1] + c_w, curr_y + LINE_HEIGHT - 2], fill=CURSOR_COLOR)
            
    return img.convert("RGB")

def highlight_command(prompt_str, cmd_str):
    tokens = [(prompt_str, PROMPT_COLOR, True)]
    if not cmd_str:
        return tokens
        
    parts = cmd_str.split(" ")
    for idx, p in enumerate(parts):
        if idx > 0:
            tokens.append((" ", TEXT_COLOR, False))
        if idx == 0 and p == "rowlite":
            tokens.append((p, CMD_COLOR, True))
        elif idx == 1 and p == "list":
            tokens.append((p, SUB_CMD_COLOR, True))
        elif p.startswith("-"):
            tokens.append((p, FLAG_COLOR, False))
        elif p.endswith(".md"):
            tokens.append((p, ARG_COLOR, False))
        else:
            tokens.append((p, TEXT_COLOR, False))
    return tokens

def parse_output_line_colors(line_text):
    t = line_text
    if "┌" in t or "├" in t or "└" in t:
        return [(t, BORDER_COLOR, False)]
    if "│" in t and (" Id " in t or " Tags " in t or " Idea " in t):
        return [(t, HEADER_COLOR, True)]
    if "│" in t:
        return [(t, TEXT_COLOR, False)]
    return [(t, TEXT_COLOR, False)]

def generate_current_state_frames():
    root_dir = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
    rowlite_bin = os.path.join(root_dir, "rowlite.exe" if sys.platform == "win32" else "rowlite")
    test_table = os.path.join(root_dir, "internal", "cmd", "testdata", "wide_ideas.table.md")
    
    # Run real command
    res = subprocess.run([rowlite_bin, "list", test_table, "--format", "pretty"], capture_output=True, text=True, encoding="utf-8")
    raw_output = res.stdout
    
    frames = []
    
    # 1. Type command at standard speed
    prompt = "❯ "
    cmd_text = "rowlite list wide_ideas.table.md --format pretty"
    f_temp = ImageDraw.Draw(Image.new("RGBA", (10, 10)))
    
    # Hold before typing (1.0s)
    for _ in range(15):
        frames.append(render_viewport([[(prompt, PROMPT_COLOR, True)]], start_line_idx=0, cursor_pos=(0, PADDING + 16)))
        
    for i in range(1, len(cmd_text) + 1, 2):
        partial = cmd_text[:i]
        hl = highlight_command(prompt, partial)
        w = PADDING
        for t, _, is_b in hl:
            w += f_temp.textbbox((0, 0), t, font=FONT_BOLD if is_b else FONT)[2]
        frames.append(render_viewport([hl], start_line_idx=0, cursor_pos=(0, w)))
        
    hl_full = highlight_command(prompt, cmd_text)
    
    # Hold after typing (0.8s)
    for _ in range(12):
        frames.append(render_viewport([hl_full], start_line_idx=0))
        
    # Build complete array of wrapped lines
    term_cols = 112
    all_lines = [hl_full]
    raw_lines = raw_output.splitlines()
    
    for raw_line in raw_lines:
        if len(raw_line) <= term_cols:
            all_lines.append(parse_output_line_colors(raw_line))
        else:
            chunks = [raw_line[i:i+term_cols] for i in range(0, len(raw_line), term_cols)]
            for c_idx, chunk in enumerate(chunks):
                if c_idx == 0:
                    all_lines.append(parse_output_line_colors(chunk))
                else:
                    all_lines.append([(chunk, WARN_COLOR if "─" not in chunk else BORDER_COLOR, False)])
                    
    total_rendered_lines = len(all_lines)
    
    # Plan pages with comfortable pauses
    # Page 1: Header breakdown (starts at 0)
    # Page 2: Middle rows breakdown (starts around 25)
    # Page 3: Long row & whitespace breakdown (starts around 50)
    # Page 4: Bottom border breakdown (starts around total_rendered_lines - MAX_VISIBLE_LINES)
    
    stops = [
        (0, "Page 1/4: Top Border & Header Wrapping", 52),                                # 3.5s
        (24, "Page 2/4: Row Wrapping & Pipe Misalignment", 52),                          # 3.5s
        (48, "Page 3/4: 1,180-Char Row & Padding Expansion", 52),                        # 3.5s
        (total_rendered_lines - MAX_VISIBLE_LINES, "Page 4/4: Trailing Whitespace & Bottom Border", 75) # 5.0s
    ]
    
    current_offset = 0
    final_frame = None
    
    for stop_idx, (target_offset, label, hold_frames) in enumerate(stops):
        # Smoothly scroll from current_offset to target_offset (slow, readable scroll)
        if target_offset > current_offset:
            # Scroll step by step: 1 line per 4 frames (~3.7 lines per second = easy to read)
            for line_pos in range(current_offset + 1, target_offset + 1):
                frame = render_viewport(all_lines, start_line_idx=line_pos, page_info="Scrolling...")
                # Repeat each step for 2 frames so motion is smooth and not a blur
                for _ in range(2):
                    frames.append(frame)
            current_offset = target_offset
            
        # Hold on page so viewer can read clearly
        page_frame = render_viewport(all_lines, start_line_idx=current_offset, page_info=label)
        final_frame = page_frame
        for _ in range(hold_frames):
            frames.append(page_frame)
            
    return frames, final_frame

def save_gif(frames, final_frame, output_paths, final_png_paths):
    temp_dir = tempfile.mkdtemp()
    try:
        for p in final_png_paths:
            os.makedirs(os.path.dirname(p), exist_ok=True)
            final_frame.save(p)
            print(f"Saved final frame PNG: {p}")

        for idx, f in enumerate(frames):
            frame_path = os.path.join(temp_dir, f"frame_{idx:04d}.png")
            f.save(frame_path)
            
        palette_path = os.path.join(temp_dir, "palette.png")
        cmd_palette = [
            "ffmpeg", "-y",
            "-framerate", str(FPS),
            "-i", os.path.join(temp_dir, "frame_%04d.png"),
            "-vf", "palettegen=stats_mode=diff",
            palette_path
        ]
        subprocess.run(cmd_palette, check=True)
        
        for out_path in output_paths:
            os.makedirs(os.path.dirname(out_path), exist_ok=True)
            cmd_gif = [
                "ffmpeg", "-y",
                "-framerate", str(FPS),
                "-i", os.path.join(temp_dir, "frame_%04d.png"),
                "-i", palette_path,
                "-lavfi", "paletteuse=dither=bayer:bayer_scale=3",
                out_path
            ]
            subprocess.run(cmd_gif, check=True)
            size_kb = os.path.getsize(out_path) / 1024
            print(f"Generated GIF: {out_path} ({size_kb:.1f} KB, {len(frames)/FPS:.1f}s)")
    finally:
        shutil.rmtree(temp_dir, ignore_errors=True)

if __name__ == "__main__":
    frames, final_frame = generate_current_state_frames()
    
    root_dir = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
    out_git = os.path.join(root_dir, "docs", "assets", "formatting-original-current.gif")
    out_dashboard = r"c:\work\dashboard\_resources\formatting-original-current.gif"
    out_artifact = r"C:\Users\pipik\.gemini\antigravity\brain\6816765f-9088-4183-826d-be7b3fb38627\formatting-original-current.gif"
    
    png_git = os.path.join(root_dir, "docs", "assets", "formatting-original-current-final.png")
    png_dashboard = r"c:\work\dashboard\_resources\formatting-original-current-final.png"
    png_artifact = r"C:\Users\pipik\.gemini\antigravity\brain\6816765f-9088-4183-826d-be7b3fb38627\formatting-original-current-final.png"

    save_gif(frames, final_frame, [out_git, out_dashboard, out_artifact], [png_git, png_dashboard, png_artifact])
