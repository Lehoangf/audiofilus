import sys
import unicodedata
from fontTools.ttLib import TTFont
from fontTools.pens.ttGlyphPen import TTGlyphPen
from fontTools.pens.transformPen import TransformPen
from PIL import Image, ImageDraw, ImageFont

def build_font(src_path, dst_path):
    font = TTFont(src_path)
    glyf = font['glyf']
    hmtx = font['hmtx']
    cmap = font.getBestCmap()

    def create_glyph(pen_callback):
        pen = TTGlyphPen(glyf)
        pen_callback(pen)
        g = pen.glyph()
        g.recalcBounds(glyf)
        return g

    # Standard hookabove (lowercase: 225x225, y=675..900)
    def draw_hookabove(pen):
        pen.moveTo((75, 675))
        pen.lineTo((150, 675))
        pen.lineTo((150, 750))
        pen.lineTo((225, 750))
        pen.lineTo((225, 900))
        pen.lineTo((0, 900))
        pen.lineTo((0, 825))
        pen.lineTo((150, 825))
        pen.lineTo((150, 750))
        pen.lineTo((75, 750))
        pen.closePath()

    # Hookabove uppercase (y=825..1050)
    def draw_hookabove_cap(pen):
        pen.moveTo((75, 825))
        pen.lineTo((150, 825))
        pen.lineTo((150, 900))
        pen.lineTo((225, 900))
        pen.lineTo((225, 1050))
        pen.lineTo((0, 1050))
        pen.lineTo((0, 975))
        pen.lineTo((150, 975))
        pen.lineTo((150, 900))
        pen.lineTo((75, 900))
        pen.closePath()

    # Compact 2-step acute for stacked accents (width 150, height 150)
    def draw_acute_compact(pen):
        # Pixel 1: (0, 0) to (75, 75)
        pen.moveTo((0, 0))
        pen.lineTo((75, 0))
        pen.lineTo((75, 75))
        pen.lineTo((0, 75))
        pen.closePath()
        # Pixel 2: (75, 75) to (150, 150)
        pen.moveTo((75, 75))
        pen.lineTo((150, 75))
        pen.lineTo((150, 150))
        pen.lineTo((75, 150))
        pen.closePath()

    # Compact 2-step grave for stacked accents (width 150, height 150)
    def draw_grave_compact(pen):
        # Pixel 1: (75, 0) to (150, 75)
        pen.moveTo((75, 0))
        pen.lineTo((150, 0))
        pen.lineTo((150, 75))
        pen.lineTo((75, 75))
        pen.closePath()
        # Pixel 2: (0, 75) to (75, 150)
        pen.moveTo((0, 75))
        pen.lineTo((75, 75))
        pen.lineTo((75, 150))
        pen.lineTo((0, 150))
        pen.closePath()

    # Compact hookabove for stacked accents (width 150, height 150)
    def draw_hook_compact(pen):
        # Top bar: (0, 75) to (150, 150)
        pen.moveTo((0, 75))
        pen.lineTo((150, 75))
        pen.lineTo((150, 150))
        pen.lineTo((0, 150))
        pen.closePath()
        # Drop stem: (75, 0) to (150, 75)
        pen.moveTo((75, 0))
        pen.lineTo((150, 0))
        pen.lineTo((150, 75))
        pen.lineTo((75, 75))
        pen.closePath()

    # Dot below (75x75 at x=0..75, y=-150..-75)
    def draw_dotbelow(pen):
        pen.moveTo((0, -150))
        pen.lineTo((0, -75))
        pen.lineTo((75, -75))
        pen.lineTo((75, -150))
        pen.closePath()

    # Horn lowercase: attached to top-right of 'o'/'u'
    def draw_horn_lc(pen):
        pen.moveTo((450, 600))
        pen.lineTo((450, 675))
        pen.lineTo((525, 675))
        pen.lineTo((525, 600))
        pen.closePath()
        pen.moveTo((525, 675))
        pen.lineTo((525, 750))
        pen.lineTo((600, 750))
        pen.lineTo((600, 675))
        pen.closePath()

    # Horn uppercase: attached to top-right of 'O'/'U'
    def draw_horn_uc(pen):
        pen.moveTo((450, 750))
        pen.lineTo((450, 825))
        pen.lineTo((525, 825))
        pen.lineTo((525, 750))
        pen.closePath()
        pen.moveTo((525, 825))
        pen.lineTo((525, 900))
        pen.lineTo((600, 900))
        pen.lineTo((600, 825))
        pen.closePath()

    # Register primitive accent glyphs in glyf
    glyf['hookabove'] = create_glyph(draw_hookabove)
    hmtx['hookabove'] = (225, 0)
    glyf['hookabove.cap'] = create_glyph(draw_hookabove_cap)
    hmtx['hookabove.cap'] = (225, 0)
    glyf['acute.compact'] = create_glyph(draw_acute_compact)
    hmtx['acute.compact'] = (150, 0)
    glyf['grave.compact'] = create_glyph(draw_grave_compact)
    hmtx['grave.compact'] = (150, 0)
    glyf['hook.compact'] = create_glyph(draw_hook_compact)
    hmtx['hook.compact'] = (150, 0)
    glyf['dotbelow'] = create_glyph(draw_dotbelow)
    hmtx['dotbelow'] = (75, 0)
    glyf['horn.lc'] = create_glyph(draw_horn_lc)
    hmtx['horn.lc'] = (600, 450)
    glyf['horn.uc'] = create_glyph(draw_horn_uc)
    hmtx['horn.uc'] = (600, 450)

    # Helper to compose glyph by drawing components with offsets
    def compose_glyph(parts, adv_width):
        pen = TTGlyphPen(glyf)
        for gname, dx, dy in parts:
            if gname in glyf:
                tpen = TransformPen(pen, (1, 0, 0, 1, dx, dy))
                glyf[gname].draw(tpen, glyf)
        res = pen.glyph()
        res.recalcBounds(glyf)
        return res, (adv_width, res.xMin if res.xMin is not None else 0)

    definitions = []

    # 1. Base 'A' / 'a'
    definitions.append(('uni1EA3', 0x1EA3, [('a', 0, 0), ('hookabove', 150, 0)], 600))
    definitions.append(('uni1EA1', 0x1EA1, [('a', 0, 0), ('dotbelow', 225, 0)], 600))
    definitions.append(('uni1EA2', 0x1EA2, [('A', 0, 0), ('hookabove.cap', 150, 0)], 600))
    definitions.append(('uni1EA0', 0x1EA0, [('A', 0, 0), ('dotbelow', 225, 0)], 600))

    # Breve 'A' / 'a' (ă, Ă)
    # Using compact accents inside/above breve bowl for crisp pixel clarity
    definitions.append(('uni1EB1', 0x1EB1, [('abreve', 0, 0), ('grave.compact', 187, 825)], 600))
    definitions.append(('uni1EAF', 0x1EAF, [('abreve', 0, 0), ('acute.compact', 187, 825)], 600))
    definitions.append(('uni1EB3', 0x1EB3, [('abreve', 0, 0), ('hook.compact', 187, 825)], 600))
    definitions.append(('uni1EB5', 0x1EB5, [('abreve', 0, 0), ('tilde', 75, 150)], 600))
    definitions.append(('uni1EB7', 0x1EB7, [('abreve', 0, 0), ('dotbelow', 225, 0)], 600))

    definitions.append(('uni1EB0', 0x1EB0, [('Abreve', 0, 0), ('grave.compact', 187, 975)], 600))
    definitions.append(('uni1EAE', 0x1EAE, [('Abreve', 0, 0), ('acute.compact', 187, 975)], 600))
    definitions.append(('uni1EB2', 0x1EB2, [('Abreve', 0, 0), ('hook.compact', 187, 975)], 600))
    definitions.append(('uni1EB4', 0x1EB4, [('Abreve', 0, 0), ('tilde', 75, 300)], 600))
    definitions.append(('uni1EB6', 0x1EB6, [('Abreve', 0, 0), ('dotbelow', 225, 0)], 600))

    # Circumflex 'A' / 'a' (â, Â)
    # Secondary accent on wings of circumflex for distinct elegance
    definitions.append(('uni1EA7', 0x1EA7, [('acircumflex', 0, 0), ('grave.compact', 112, 825)], 600))
    definitions.append(('uni1EA5', 0x1EA5, [('acircumflex', 0, 0), ('acute.compact', 262, 825)], 600))
    definitions.append(('uni1EA9', 0x1EA9, [('acircumflex', 0, 0), ('hook.compact', 262, 825)], 600))
    definitions.append(('uni1EAB', 0x1EAB, [('acircumflex', 0, 0), ('tilde', 75, 150)], 600))
    definitions.append(('uni1EAD', 0x1EAD, [('acircumflex', 0, 0), ('dotbelow', 225, 0)], 600))

    definitions.append(('uni1EA6', 0x1EA6, [('Acircumflex', 0, 0), ('grave.compact', 112, 975)], 600))
    definitions.append(('uni1EA4', 0x1EA4, [('Acircumflex', 0, 0), ('acute.compact', 262, 975)], 600))
    definitions.append(('uni1EA8', 0x1EA8, [('Acircumflex', 0, 0), ('hook.compact', 262, 975)], 600))
    definitions.append(('uni1EAA', 0x1EAA, [('Acircumflex', 0, 0), ('tilde', 75, 300)], 600))
    definitions.append(('uni1EAC', 0x1EAC, [('Acircumflex', 0, 0), ('dotbelow', 225, 0)], 600))

    # 2. 'E' / 'e'
    definitions.append(('uni1EBB', 0x1EBB, [('e', 0, 0), ('hookabove', 150, 0)], 600))
    definitions.append(('uni1EBD', 0x1EBD, [('e', 0, 0), ('tilde', 75, 0)], 600))
    definitions.append(('uni1EB9', 0x1EB9, [('e', 0, 0), ('dotbelow', 225, 0)], 600))
    definitions.append(('uni1EBA', 0x1EBA, [('E', 0, 0), ('hookabove.cap', 112, 0)], 525))
    definitions.append(('uni1EBC', 0x1EBC, [('E', 0, 0), ('tilde', 75, 150)], 525))
    definitions.append(('uni1EB8', 0x1EB8, [('E', 0, 0), ('dotbelow', 187, 0)], 525))

    # Circumflex 'E' / 'e' (ê, Ê)
    definitions.append(('uni1EC1', 0x1EC1, [('ecircumflex', 0, 0), ('grave.compact', 112, 825)], 600))
    definitions.append(('uni1EBF', 0x1EBF, [('ecircumflex', 0, 0), ('acute.compact', 262, 825)], 600))
    definitions.append(('uni1EC3', 0x1EC3, [('ecircumflex', 0, 0), ('hook.compact', 262, 825)], 600))
    definitions.append(('uni1EC5', 0x1EC5, [('ecircumflex', 0, 0), ('tilde', 75, 150)], 600))
    definitions.append(('uni1EC7', 0x1EC7, [('ecircumflex', 0, 0), ('dotbelow', 225, 0)], 600))

    definitions.append(('uni1EC0', 0x1EC0, [('Ecircumflex', 0, 0), ('grave.compact', 112, 975)], 525))
    definitions.append(('uni1EBE', 0x1EBE, [('Ecircumflex', 0, 0), ('acute.compact', 262, 975)], 525))
    definitions.append(('uni1EC2', 0x1EC2, [('Ecircumflex', 0, 0), ('hook.compact', 262, 975)], 525))
    definitions.append(('uni1EC4', 0x1EC4, [('Ecircumflex', 0, 0), ('tilde', 75, 300)], 525))
    definitions.append(('uni1EC6', 0x1EC6, [('Ecircumflex', 0, 0), ('dotbelow', 187, 0)], 525))

    # 3. 'I' / 'i'
    definitions.append(('uni1EC9', 0x1EC9, [('dotlessi', 0, 0), ('hookabove', 0, 0)], 225))
    definitions.append(('uni0129', 0x0129, [('dotlessi', 0, 0), ('tilde', -75, 0)], 225))
    definitions.append(('uni1ECB', 0x1ECB, [('i', 0, 0), ('dotbelow', 37, 0)], 225))

    definitions.append(('uni1EC8', 0x1EC8, [('I', 0, 0), ('hookabove.cap', -37, 0)], 225))
    definitions.append(('uni0128', 0x0128, [('I', 0, 0), ('tilde', -75, 150)], 225))
    definitions.append(('uni1ECA', 0x1ECA, [('I', 0, 0), ('dotbelow', 37, 0)], 225))

    # 4. 'O' / 'o'
    definitions.append(('uni1ECF', 0x1ECF, [('o', 0, 0), ('hookabove', 150, 0)], 600))
    definitions.append(('uni1ECD', 0x1ECD, [('o', 0, 0), ('dotbelow', 225, 0)], 600))
    definitions.append(('uni1ECE', 0x1ECE, [('O', 0, 0), ('hookabove.cap', 150, 0)], 600))
    definitions.append(('uni1ECC', 0x1ECC, [('O', 0, 0), ('dotbelow', 225, 0)], 600))

    # Circumflex 'O' / 'o' (ô, Ô)
    definitions.append(('uni1ED3', 0x1ED3, [('ocircumflex', 0, 0), ('grave.compact', 112, 825)], 600))
    definitions.append(('uni1ED1', 0x1ED1, [('ocircumflex', 0, 0), ('acute.compact', 262, 825)], 600))
    definitions.append(('uni1ED5', 0x1ED5, [('ocircumflex', 0, 0), ('hook.compact', 262, 825)], 600))
    definitions.append(('uni1ED7', 0x1ED7, [('ocircumflex', 0, 0), ('tilde', 75, 150)], 600))
    definitions.append(('uni1ED9', 0x1ED9, [('ocircumflex', 0, 0), ('dotbelow', 225, 0)], 600))

    definitions.append(('uni1ED2', 0x1ED2, [('Ocircumflex', 0, 0), ('grave.compact', 112, 975)], 600))
    definitions.append(('uni1ED0', 0x1ED0, [('Ocircumflex', 0, 0), ('acute.compact', 262, 975)], 600))
    definitions.append(('uni1ED4', 0x1ED4, [('Ocircumflex', 0, 0), ('hook.compact', 262, 975)], 600))
    definitions.append(('uni1ED6', 0x1ED6, [('Ocircumflex', 0, 0), ('tilde', 75, 300)], 600))
    definitions.append(('uni1ED8', 0x1ED8, [('Ocircumflex', 0, 0), ('dotbelow', 225, 0)], 600))

    # Horn 'O' / 'o' (ơ, Ơ)
    definitions.append(('uni01A1', 0x01A1, [('o', 0, 0), ('horn.lc', 0, 0)], 600))
    definitions.append(('uni1EDD', 0x1EDD, [('o', 0, 0), ('horn.lc', 0, 0), ('grave', 112, 0)], 600))
    definitions.append(('uni1EDB', 0x1EDB, [('o', 0, 0), ('horn.lc', 0, 0), ('acute', 112, 0)], 600))
    definitions.append(('uni1EDF', 0x1EDF, [('o', 0, 0), ('horn.lc', 0, 0), ('hookabove', 112, 0)], 600))
    definitions.append(('uni1EE1', 0x1EE1, [('o', 0, 0), ('horn.lc', 0, 0), ('tilde', 75, 0)], 600))
    definitions.append(('uni1EE3', 0x1EE3, [('o', 0, 0), ('horn.lc', 0, 0), ('dotbelow', 225, 0)], 600))

    definitions.append(('uni01A0', 0x01A0, [('O', 0, 0), ('horn.uc', 0, 0)], 600))
    definitions.append(('uni1EDC', 0x1EDC, [('O', 0, 0), ('horn.uc', 0, 0), ('grave', 112, 150)], 600))
    definitions.append(('uni1EDA', 0x1EDA, [('O', 0, 0), ('horn.uc', 0, 0), ('acute', 112, 150)], 600))
    definitions.append(('uni1EDE', 0x1EDE, [('O', 0, 0), ('horn.uc', 0, 0), ('hookabove.cap', 112, 0)], 600))
    definitions.append(('uni1EE0', 0x1EE0, [('O', 0, 0), ('horn.uc', 0, 0), ('tilde', 75, 150)], 600))
    definitions.append(('uni1EE2', 0x1EE2, [('O', 0, 0), ('horn.uc', 0, 0), ('dotbelow', 225, 0)], 600))

    # 5. 'U' / 'u'
    definitions.append(('uni1EE7', 0x1EE7, [('u', 0, 0), ('hookabove', 150, 0)], 600))
    definitions.append(('uni0169', 0x0169, [('u', 0, 0), ('tilde', 75, 0)], 600))
    definitions.append(('uni1EE5', 0x1EE5, [('u', 0, 0), ('dotbelow', 225, 0)], 600))

    definitions.append(('uni1EE6', 0x1EE6, [('U', 0, 0), ('hookabove.cap', 150, 0)], 600))
    definitions.append(('uni0168', 0x0168, [('U', 0, 0), ('tilde', 75, 150)], 600))
    definitions.append(('uni1EE4', 0x1EE4, [('U', 0, 0), ('dotbelow', 225, 0)], 600))

    # Horn 'U' / 'u' (ư, Ư)
    definitions.append(('uni01B0', 0x01B0, [('u', 0, 0), ('horn.lc', 0, 0)], 600))
    definitions.append(('uni1EEB', 0x1EEB, [('u', 0, 0), ('horn.lc', 0, 0), ('grave', 112, 0)], 600))
    definitions.append(('uni1EE9', 0x1EE9, [('u', 0, 0), ('horn.lc', 0, 0), ('acute', 112, 0)], 600))
    definitions.append(('uni1EED', 0x1EED, [('u', 0, 0), ('horn.lc', 0, 0), ('hookabove', 112, 0)], 600))
    definitions.append(('uni1EEF', 0x1EEF, [('u', 0, 0), ('horn.lc', 0, 0), ('tilde', 75, 0)], 600))
    definitions.append(('uni1EF1', 0x1EF1, [('u', 0, 0), ('horn.lc', 0, 0), ('dotbelow', 225, 0)], 600))

    definitions.append(('uni01AF', 0x01AF, [('U', 0, 0), ('horn.uc', 0, 0)], 600))
    definitions.append(('uni1EEA', 0x1EEA, [('U', 0, 0), ('horn.uc', 0, 0), ('grave', 112, 150)], 600))
    definitions.append(('uni1EE8', 0x1EE8, [('U', 0, 0), ('horn.uc', 0, 0), ('acute', 112, 150)], 600))
    definitions.append(('uni1EEC', 0x1EEC, [('U', 0, 0), ('horn.uc', 0, 0), ('hookabove.cap', 112, 0)], 600))
    definitions.append(('uni1EEE', 0x1EEE, [('U', 0, 0), ('horn.uc', 0, 0), ('tilde', 75, 150)], 600))
    definitions.append(('uni1EF0', 0x1EF0, [('U', 0, 0), ('horn.uc', 0, 0), ('dotbelow', 225, 0)], 600))

    # 6. 'Y' / 'y'
    definitions.append(('uni1EF7', 0x1EF7, [('y', 0, 0), ('hookabove', 112, 0)], 525))
    definitions.append(('uni1EF9', 0x1EF9, [('y', 0, 0), ('tilde', 75, 0)], 525))
    definitions.append(('uni1EF5', 0x1EF5, [('y', 0, 0), ('dotbelow', 187, -150)], 525))

    definitions.append(('uni1EF6', 0x1EF6, [('Y', 0, 0), ('hookabove.cap', 187, 0)], 675))
    definitions.append(('uni1EF8', 0x1EF8, [('Y', 0, 0), ('tilde', 112, 150)], 675))
    definitions.append(('uni1EF4', 0x1EF4, [('Y', 0, 0), ('dotbelow', 262, 0)], 675))

    # Build glyphs and add to cmap
    order = font.getGlyphOrder()
    for gname, cp, parts, adv_width in definitions:
        g, metrics = compose_glyph(parts, adv_width)
        glyf[gname] = g
        hmtx[gname] = metrics
        cmap[cp] = gname
        if gname not in order:
            order.append(gname)

    font.setGlyphOrder(order)
    font.save(dst_path)
    print(f'Successfully built {dst_path} with {len(definitions)} new Vietnamese glyphs!')

if __name__ == '__main__':
    src = 'internal/ui/assets/fonts/Jersey10-Regular.ttf'
    dst = 'dist/Jersey10-Vietnamese.ttf'
    build_font(src, dst)
