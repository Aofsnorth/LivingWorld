import com.mojang.datafixers.util.Pair;
import net.minecraft.resources.ResourceKey;
import net.minecraft.world.level.biome.Biome;
import net.minecraft.world.level.biome.Climate;
import net.minecraft.world.level.biome.MultiNoiseBiomeSourceParameterList;

/**
 * Dumps the vanilla 26.3 Overworld multi-noise biome parameter list — the
 * Climate.ParameterPoint entries OverworldBiomeBuilder produces — as JSON
 * with RAW quantized coordinates (longs, quantizeCoord = (long)(v*10000f)),
 * in declaration order (the order Climate.ParameterList.findValueBruteForce
 * breaks ties with). This is the source of truth for the Go biome source.
 */
public class BiomeDump {
    public static void main(String[] args) {
        var presets = MultiNoiseBiomeSourceParameterList.knownPresets();
        var list = presets.get(MultiNoiseBiomeSourceParameterList.Preset.OVERWORLD);
        StringBuilder sb = new StringBuilder();
        sb.append("[\n");
        boolean first = true;
        int count = 0;
        for (Pair<Climate.ParameterPoint, ResourceKey<Biome>> e : list.values()) {
            Climate.ParameterPoint pp = e.getFirst();
            if (!first) {
                sb.append(",\n");
            }
            first = false;
            count++;
            sb.append(String.format(
                "  {\"biome\": \"%s\", \"t\": [%d, %d], \"h\": [%d, %d], \"c\": [%d, %d], \"e\": [%d, %d], \"d\": [%d, %d], \"w\": [%d, %d], \"o\": %d}",
                e.getSecond().identifier(),
                pp.temperature().min(), pp.temperature().max(),
                pp.humidity().min(), pp.humidity().max(),
                pp.continentalness().min(), pp.continentalness().max(),
                pp.erosion().min(), pp.erosion().max(),
                pp.depth().min(), pp.depth().max(),
                pp.weirdness().min(), pp.weirdness().max(),
                pp.offset()));
        }
        sb.append("\n]\n");
        System.out.print(sb);
        System.err.println("entries: " + count);
    }
}
