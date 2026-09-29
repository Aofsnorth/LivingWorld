import net.minecraft.SharedConstants;
import net.minecraft.core.Holder;
import net.minecraft.core.HolderLookup;
import net.minecraft.core.registries.Registries;
import net.minecraft.resources.Identifier;
import net.minecraft.resources.ResourceKey;
import net.minecraft.server.Bootstrap;
import net.minecraft.world.level.levelgen.NoiseGeneratorSettings;
import net.minecraft.world.level.levelgen.NoiseRouter;
import net.minecraft.world.level.levelgen.RandomState;
import net.minecraft.world.level.levelgen.densityfunction.DensityBufferPool;
import net.minecraft.world.level.levelgen.densityfunction.DensityFunction;
import net.minecraft.world.level.levelgen.densityfunction.SamplerContext;

import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;

/**
 * RouterOracle — vanilla 26.3 ground truth for the eight noise-router fields
 * at exact block coordinates, so LivingWorld's density-function evaluation can
 * be diffed field by field against the real implementation:
 *
 *   <x> <y> <z> <shiftX> <shiftZ> <inX> <inY> <inZ> <noiseAtInput>
 *        <temperature> <vegetation> <continents>
 *        <erosion> <depth> <ridges> <chunkSurfaceLevel> <finalDensity>
 *
 * Usage: RouterOracle <seed> <in.txt> <out.txt>
 */
public class RouterOracle {
    public static void main(String[] args) throws Exception {
        if (args.length < 3) {
            System.out.println("usage: RouterOracle <seed> <in.txt> <out.txt>");
            System.exit(2);
        }
        long seed = Long.parseLong(args[0]);
        SharedConstants.tryDetectVersion();
        Bootstrap.bootStrap();

        HolderLookup.Provider lookup = net.minecraft.data.registries.VanillaRegistries.createWorldLookup();
        Holder<NoiseGeneratorSettings> settingsHolder = lookup.lookupOrThrow(Registries.NOISE_SETTINGS)
                .getOrThrow(ResourceKey.create(Registries.NOISE_SETTINGS, Identifier.parse("minecraft:overworld")));
        NoiseGeneratorSettings settings = settingsHolder.value();
        NoiseRouter router = settings.noiseRouter();

        // The climate shift functions, as the vanilla shifted_noise sees them.
        var densityLookup = lookup.lookupOrThrow(Registries.DENSITY_FUNCTION);
        var shiftXKey = ResourceKey.create(Registries.DENSITY_FUNCTION,
                Identifier.parse("minecraft:shift_x"));
        var shiftZKey = ResourceKey.create(Registries.DENSITY_FUNCTION,
                Identifier.parse("minecraft:shift_z"));
        var shiftX = densityLookup.getOrThrow(shiftXKey).value();
        var shiftZ = densityLookup.getOrThrow(shiftZKey).value();

        RandomState randomState = RandomState.create(lookup.lookupOrThrow(Registries.NOISE), seed, settings);
        DensityBufferPool pool = randomState.acquireDensityBufferPool();
        SamplerContext ctx = SamplerContext.builder().enableCaches().useBufferArena(pool).build();

        StringBuilder sb = new StringBuilder();
        for (String line : Files.readAllLines(Path.of(args[1]))) {
            String[] p = line.trim().split("\\s+");
            if (p.length < 3) continue;
            int x = Integer.parseInt(p[0]), y = Integer.parseInt(p[1]), z = Integer.parseInt(p[2]);
            sb.append(x).append(' ').append(y).append(' ').append(z);
            sb.append(' ').append(randomState.sampleBlockValueUncached(shiftX, x, y, z));
            sb.append(' ').append(randomState.sampleBlockValueUncached(shiftZ, x, y, z));
            // The shifted continentalness input and the raw noise sampled there.
            float sx = randomState.sampleBlockValueUncached(shiftX, x, y, z);
            float sz = randomState.sampleBlockValueUncached(shiftZ, x, y, z);
            var contNoiseObj = randomState.getOrCreateNoise(
                    ResourceKey.create(Registries.NOISE, Identifier.parse("minecraft:continentalness")));
            double inX = x * 0.25 + sx;
            double inY = y * 0.0;
            double inZ = z * 0.25 + sz;
            sb.append(' ').append(String.format(java.util.Locale.ROOT, "%.17g", inX))
                    .append(' ').append(String.format(java.util.Locale.ROOT, "%.17g", inY))
                    .append(' ').append(String.format(java.util.Locale.ROOT, "%.17g", inZ))
                    .append(' ').append(contNoiseObj.get(inX, inY, inZ));
            for (DensityFunction f : new DensityFunction[]{
                    router.temperature(), router.vegetation(), router.continents(), router.erosion(),
                    router.depth(), router.ridges(), router.chunkSurfaceLevel(), router.finalDensity()}) {
                sb.append(' ').append(randomState.sampleBlockValueUncached(f, x, y, z));
            }
            sb.append('\n');
        }
        Files.write(Path.of(args[2]), sb.toString().getBytes(StandardCharsets.UTF_8));
        randomState.releaseDensityBufferPool(pool);
        System.out.println("router oracle done: " + args[1]);
    }
}
